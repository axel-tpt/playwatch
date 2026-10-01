// Package ansible wraps the ansible-* command line tools. It never parses
// Ansible files itself: every piece of data comes from Ansible's JSON output.
// It must not depend on any UI package.
package ansible

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// DefaultTimeout bounds a single ansible-* invocation. Dynamic inventories can
// be slow, so it is generous.
const DefaultTimeout = 2 * time.Minute

// ErrNotInstalled is returned when an ansible-* binary is not in $PATH.
var ErrNotInstalled = errors.New("not found in $PATH (is Ansible installed?)")

// Client runs ansible-* commands against a project.
type Client struct {
	// Inventories are passed as repeated -i flags. When empty, Ansible falls
	// back to its own configuration (ansible.cfg, ANSIBLE_INVENTORY).
	Inventories []string
	// Dir is the working directory of the commands, so that the project's
	// ansible.cfg is picked up. Empty means the current directory.
	Dir string
	// Env holds extra KEY=VALUE entries appended to the process environment.
	Env []string
	// Timeout bounds each command. Zero disables it.
	Timeout time.Duration
}

// NewClient returns a Client using the given inventories and DefaultTimeout.
func NewClient(inventories ...string) *Client {
	return &Client{Inventories: inventories, Timeout: DefaultTimeout}
}

// CommandError reports a failed ansible-* invocation. Stderr is kept verbatim
// so it can be shown to the user as is.
type CommandError struct {
	Name   string
	Args   []string
	Stderr string
	Err    error
}

func (e *CommandError) Error() string {
	msg := strings.TrimSpace(e.Stderr)
	if msg == "" {
		return fmt.Sprintf("%s: %v", e.Name, e.Err)
	}
	return fmt.Sprintf("%s: %v\n%s", e.Name, e.Err, msg)
}

func (e *CommandError) Unwrap() error { return e.Err }

// output runs an ansible-* command and returns its stdout and stderr. Ansible
// often succeeds while printing warnings (e.g. an unparsable inventory source
// still exits 0), so stderr is returned on success too.
func (c *Client) output(ctx context.Context, name string, args ...string) ([]byte, string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", name, ErrNotInstalled)
	}
	if c.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Dir = c.Dir
	cmd.Env = c.environ()
	cmd.WaitDelay = 5 * time.Second
	// Ansible refuses non-blocking stdio. A nil Stdin is /dev/null and buffers
	// are fed through pipes whose child end is blocking.
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			err = ctxErr
		}
		return nil, stderr.String(), &CommandError{Name: name, Args: args, Stderr: stderr.String(), Err: err}
	}
	return stdout.Bytes(), stderr.String(), nil
}

func (c *Client) environ() []string {
	env := append(os.Environ(), "ANSIBLE_NOCOLOR=1", "ANSIBLE_FORCE_COLOR=0")
	return append(env, c.Env...)
}

func (c *Client) inventoryArgs() []string {
	args := make([]string, 0, 2*len(c.Inventories))
	for _, inv := range c.Inventories {
		args = append(args, "-i", inv)
	}
	return args
}
