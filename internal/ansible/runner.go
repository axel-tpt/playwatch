package ansible

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/axel-tpt/playwatch/internal/ansible/callback"
)

const (
	// maxEventSize bounds a single event line (a module result can be big).
	maxEventSize = 64 << 20
	// stderrTail is how much of stderr is kept for error reporting.
	stderrTail = 64 << 10
	// drainTimeout is how long to wait for the end of the event stream
	// after ansible-playbook exited.
	drainTimeout = 2 * time.Second
)

// PlaybookOptions describes an ansible-playbook run.
type PlaybookOptions struct {
	Playbook string
	// Args are extra ansible-playbook arguments (e.g. "--limit", "web").
	Args []string
	// Output receives ansible-playbook's raw stdout and stderr. Writes are
	// serialized. Nil discards them.
	Output io.Writer
	// VaultPassword, when set, is handed to Ansible through a pipe
	// (--vault-password-file /dev/fd/4): it never touches the disk. The
	// caller may wipe it once RunPlaybook has returned.
	VaultPassword []byte
	// Color makes Ansible emit ANSI colors in Output even though it is not a
	// terminal. Leave it off when the output goes to a file or a pipe.
	Color bool
}

// vaultPasswordFD is the child's file descriptor carrying the vault password.
const vaultPasswordFD = 4

// Run is a running ansible-playbook process.
type Run struct {
	// Events streams the events of the run. It must be drained until it is
	// closed, which happens once the process has exited: a consumer that
	// stops reading blocks the run.
	Events <-chan Event

	cmd    *exec.Cmd
	stderr *tailBuffer
	done   chan struct{}
	err    error
}

// RunPlaybook starts ansible-playbook with the playwatch_events callback enabled.
// Cancelling ctx interrupts the run (SIGINT to the whole process group, then
// SIGKILL if it does not exit in time).
//
// TODO: ANSIBLE_CALLBACK_PLUGINS and ANSIBLE_CALLBACKS_ENABLED override the
// values set in ansible.cfg; merge them with `ansible-config dump`.
func (c *Client) RunPlaybook(ctx context.Context, opts PlaybookOptions) (*Run, error) {
	path, err := exec.LookPath("ansible-playbook")
	if err != nil {
		return nil, fmt.Errorf("ansible-playbook: %w", ErrNotInstalled)
	}
	pluginDir, cleanup, err := callback.Install()
	if err != nil {
		return nil, err
	}
	pr, pw, err := os.Pipe()
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("create events pipe: %w", err)
	}

	args := append(c.inventoryArgs(), opts.Args...)
	var vaultR, vaultW *os.File
	if len(opts.VaultPassword) > 0 {
		if vaultR, vaultW, err = os.Pipe(); err != nil {
			_ = pr.Close()
			_ = pw.Close()
			cleanup()
			return nil, fmt.Errorf("create vault password pipe: %w", err)
		}
		args = append(args, "--vault-password-file", fmt.Sprintf("/dev/fd/%d", vaultPasswordFD))
	}
	args = append(args, opts.Playbook)
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Dir = c.Dir
	cmd.Env = append(c.environ(),
		"ANSIBLE_CALLBACK_PLUGINS="+pluginDir,
		"ANSIBLE_CALLBACKS_ENABLED="+callback.Name,
		"PLAYWATCH_EVENTS_FD=3", // ExtraFiles[0] is fd 3 in the child
	)
	if opts.Color {
		cmd.Env = append(cmd.Env, "ANSIBLE_NOCOLOR=0", "ANSIBLE_FORCE_COLOR=1")
	}
	cmd.ExtraFiles = []*os.File{pw} // fd 3
	if vaultR != nil {
		cmd.ExtraFiles = append(cmd.ExtraFiles, vaultR) // fd 4
	}
	cmd.WaitDelay = 10 * time.Second
	setProcessGroup(cmd)

	out := opts.Output
	if out == nil {
		out = io.Discard
	}
	out = &lockedWriter{w: out}
	stderr := &tailBuffer{max: stderrTail}
	cmd.Stdout = out
	cmd.Stderr = io.MultiWriter(out, stderr)

	if err := cmd.Start(); err != nil {
		_ = pr.Close()
		_ = pw.Close()
		closeAll(vaultR, vaultW)
		cleanup()
		return nil, fmt.Errorf("start ansible-playbook: %w", err)
	}
	// Only the child must hold the write end, so that EOF means it is done.
	_ = pw.Close()
	if vaultW != nil {
		_ = vaultR.Close()
		// The caller may wipe its password as soon as RunPlaybook returns:
		// write a private copy, and wipe it once written.
		password := bytes.Clone(opts.VaultPassword)
		go func() {
			// Ansible reads the password once; a short write only means it
			// exited early, which the run reports anyway.
			_, _ = vaultW.Write(password)
			_ = vaultW.Close()
			clear(password)
		}()
	}

	events := make(chan Event, 64)
	r := &Run{Events: events, cmd: cmd, stderr: stderr, done: make(chan struct{})}
	go r.supervise(pr, events, cleanup)
	return r, nil
}

func (r *Run) supervise(pr *os.File, events chan<- Event, cleanup func()) {
	defer close(r.done)
	defer cleanup()

	readDone := make(chan error, 1)
	go func() { readDone <- readEvents(pr, events) }()

	waitErr := r.cmd.Wait()
	var readErr error
	select {
	case readErr = <-readDone:
	case <-time.After(drainTimeout):
		// A leftover process still holds the pipe: stop reading.
		_ = pr.Close()
		readErr = <-readDone
	}
	_ = pr.Close()
	close(events)

	if waitErr != nil {
		waitErr = &CommandError{Name: "ansible-playbook", Args: r.cmd.Args[1:], Stderr: r.stderr.String(), Err: waitErr}
	}
	if readErr != nil && !errors.Is(readErr, os.ErrClosed) {
		readErr = fmt.Errorf("read events: %w", readErr)
	} else {
		readErr = nil
	}
	r.err = errors.Join(waitErr, readErr)
}

// readEvents forwards each line of r to events. Malformed lines are skipped
// and the first parse error is returned at the end.
func readEvents(r io.Reader, events chan<- Event) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), maxEventSize)
	var parseErr error
	for sc.Scan() {
		e, err := ParseEvent(sc.Bytes())
		if err != nil {
			if parseErr == nil {
				parseErr = err
			}
			continue
		}
		events <- e
	}
	return errors.Join(sc.Err(), parseErr)
}

// Wait blocks until the run is over and its events channel is closed. The
// error is a *CommandError when ansible-playbook exited with a non-zero code.
func (r *Run) Wait() error {
	<-r.done
	return r.err
}

// Done is closed once the run is over.
func (r *Run) Done() <-chan struct{} { return r.done }

// ExitCode returns ansible-playbook's exit code, or -1 while it is running
// or if it was killed by a signal. Ansible uses 2 for failed hosts and 4 for
// unreachable ones.
func (r *Run) ExitCode() int {
	select {
	case <-r.done:
		return r.cmd.ProcessState.ExitCode()
	default:
		return -1
	}
}

func closeAll(files ...*os.File) {
	for _, f := range files {
		if f != nil {
			_ = f.Close()
		}
	}
}

type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// tailBuffer keeps the last max bytes written to it.
type tailBuffer struct {
	mu  sync.Mutex
	max int
	buf []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if over := len(t.buf) - t.max; over > 0 {
		t.buf = append(t.buf[:0], t.buf[over:]...)
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}
