// Package store persists playbook runs under $XDG_STATE_HOME/playwatch. Each run is
// three files: <id>.json (metadata), <id>.jsonl (events, one per line, as
// emitted by the callback plugin) and <id>.log (ansible-playbook's raw
// output). It must not depend on any UI package.
package store

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/axel-tpt/playwatch/internal/ansible"
)

// Run states as stored in metadata.
const (
	StateRunning     = "running"
	StateOK          = "ok"
	StateFailed      = "failed"
	StateInterrupted = "interrupted"
	// StateAborted is reported for runs left "running" by an playwatch process
	// that no longer exists.
	StateAborted = "aborted"
)

// Meta describes a stored run.
type Meta struct {
	ID          string              `json:"id"`
	Playbook    string              `json:"playbook"`
	Project     string              `json:"project"` // absolute project directory
	Inventories []string            `json:"inventories,omitempty"`
	Options     *ansible.RunOptions `json:"options,omitempty"`
	// Args are the ansible-playbook arguments, for display. Runs recorded
	// before options existed only have these.
	Args     []string                     `json:"args,omitempty"`
	Started  time.Time                    `json:"started"`
	Finished time.Time                    `json:"finished,omitzero"`
	State    string                       `json:"state"`
	ExitCode int                          `json:"exit_code"`
	Stats    map[string]ansible.HostStats `json:"stats,omitempty"`
	PID      int                          `json:"pid"` // playwatch process that ran it
}

// RunOptions returns the options of the run.
func (m Meta) RunOptions() ansible.RunOptions {
	if m.Options != nil {
		return *m.Options
	}
	return ansible.RunOptions{Extra: m.Args}
}

// Duration returns how long the run took (so far, if it is still running).
func (m Meta) Duration() time.Duration {
	if m.Finished.IsZero() {
		return time.Since(m.Started)
	}
	return m.Finished.Sub(m.Started)
}

// Store is a directory of runs.
type Store struct {
	dir string
}

// DefaultDir returns $XDG_STATE_HOME/playwatch/runs, falling back to
// ~/.local/state/playwatch/runs.
func DefaultDir() (string, error) {
	state := os.Getenv("XDG_STATE_HOME")
	if state == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("locate state directory: %w", err)
		}
		state = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(state, "playwatch", "runs"), nil
}

// Open opens (and creates if needed) a store. Run results can hold sensitive
// data, so the directory is private to the user.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("open run store: %w", err)
	}
	return &Store{dir: dir}, nil
}

// Dir returns the store directory.
func (s *Store) Dir() string { return s.dir }

// Create starts recording a new run. meta.ID, State, PID and Started (if
// zero) are filled in.
func (s *Store) Create(meta Meta) (*Recorder, error) {
	id, err := newID(time.Now())
	if err != nil {
		return nil, err
	}
	meta.ID = id
	meta.State = StateRunning
	meta.ExitCode = -1
	meta.PID = os.Getpid()
	if meta.Started.IsZero() {
		meta.Started = time.Now()
	}
	if err := s.writeMeta(meta); err != nil {
		return nil, err
	}
	events, err := os.OpenFile(s.path(id, ".jsonl"), os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create run events: %w", err)
	}
	output, err := os.OpenFile(s.path(id, ".log"), os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if err != nil {
		_ = events.Close()
		return nil, fmt.Errorf("create run output: %w", err)
	}
	return &Recorder{store: s, meta: meta, events: events, output: output}, nil
}

// List returns the runs of a project (all projects if project is empty),
// most recent first.
func (s *Store) List(project string) ([]Meta, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	var metas []Meta
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok || e.IsDir() {
			continue
		}
		meta, err := s.Meta(id)
		if err != nil {
			continue // a half-written or foreign file must not hide the others
		}
		if project == "" || meta.Project == project {
			metas = append(metas, meta)
		}
	}
	slices.SortFunc(metas, func(a, b Meta) int { return b.Started.Compare(a.Started) })
	return metas, nil
}

// Meta reads a run's metadata. A run still marked running whose playwatch process
// is gone is reported as aborted.
func (s *Store) Meta(id string) (Meta, error) {
	data, err := os.ReadFile(s.path(id, ".json"))
	if err != nil {
		return Meta{}, fmt.Errorf("read run %s: %w", id, err)
	}
	var meta Meta
	if err := json.Unmarshal(data, &meta); err != nil {
		return Meta{}, fmt.Errorf("parse run %s: %w", id, err)
	}
	if meta.State == StateRunning && !processAlive(meta.PID) {
		meta.State = StateAborted
	}
	return meta, nil
}

// Events reads the recorded events of a run. A truncated last line (playwatch
// killed mid-write) is ignored.
func (s *Store) Events(id string) ([]ansible.Event, error) {
	f, err := os.Open(s.path(id, ".jsonl"))
	if err != nil {
		return nil, fmt.Errorf("read run %s events: %w", id, err)
	}
	defer func() { _ = f.Close() }()

	var events []ansible.Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 64<<20)
	for sc.Scan() {
		e, err := ansible.ParseEvent(sc.Bytes())
		if err != nil {
			continue
		}
		events = append(events, e)
	}
	if err := sc.Err(); err != nil {
		return events, fmt.Errorf("read run %s events: %w", id, err)
	}
	return events, nil
}

// Output returns the raw ansible-playbook output of a run.
func (s *Store) Output(id string) (string, error) {
	data, err := os.ReadFile(s.path(id, ".log"))
	if err != nil {
		return "", fmt.Errorf("read run %s output: %w", id, err)
	}
	return string(data), nil
}

// Delete removes a run.
func (s *Store) Delete(id string) error {
	var errs []error
	for _, ext := range []string{".json", ".jsonl", ".log"} {
		if err := os.Remove(s.path(id, ext)); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("delete run %s: %w", id, err)
	}
	return nil
}

// Prune keeps the keep most recent finished runs (all projects) and deletes
// the others. Runs in progress are never deleted.
func (s *Store) Prune(keep int) error {
	metas, err := s.List("")
	if err != nil {
		return err
	}
	var errs []error
	kept := 0
	for _, m := range metas {
		if m.State == StateRunning {
			continue
		}
		if kept < keep {
			kept++
			continue
		}
		errs = append(errs, s.Delete(m.ID))
	}
	return errors.Join(errs...)
}

func (s *Store) path(id, ext string) string {
	return filepath.Join(s.dir, id+ext)
}

// writeMeta replaces the metadata file atomically.
func (s *Store) writeMeta(meta Meta) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("encode run %s: %w", meta.ID, err)
	}
	tmp, err := os.CreateTemp(s.dir, ".meta-*")
	if err != nil {
		return fmt.Errorf("write run %s: %w", meta.ID, err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("write run %s: %w", meta.ID, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("write run %s: %w", meta.ID, err)
	}
	if err := os.Rename(tmp.Name(), s.path(meta.ID, ".json")); err != nil {
		return fmt.Errorf("write run %s: %w", meta.ID, err)
	}
	return nil
}

// newID returns a sortable, unique run ID such as 20261001-224529-3fa2.
func newID(t time.Time) (string, error) {
	var b [2]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate run id: %w", err)
	}
	return t.Format("20060102-150405") + "-" + hex.EncodeToString(b[:]), nil
}

// Recorder writes one run to the store. Record and Output may be called from
// different goroutines, but Record calls must not be concurrent.
type Recorder struct {
	store  *Store
	meta   Meta
	events *os.File
	output *os.File
}

// Meta returns the metadata the run was created with.
func (r *Recorder) Meta() Meta { return r.meta }

// Record appends an event.
func (r *Recorder) Record(e ansible.Event) error {
	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("encode event: %w", err)
	}
	if _, err := r.events.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("record event: %w", err)
	}
	return nil
}

// Output is where ansible-playbook's raw output is saved. Write errors are
// ignored, so that a full disk never breaks ansible-playbook's output pipe.
func (r *Recorder) Output() io.Writer { return bestEffort{r.output} }

type bestEffort struct{ w io.Writer }

func (b bestEffort) Write(p []byte) (int, error) {
	_, _ = b.w.Write(p)
	return len(p), nil
}

// Finish closes the run's files and records its final state.
func (r *Recorder) Finish(state string, exitCode int, stats map[string]ansible.HostStats) error {
	r.meta.State = state
	r.meta.ExitCode = exitCode
	r.meta.Stats = stats
	r.meta.Finished = time.Now()
	return errors.Join(r.events.Close(), r.output.Close(), r.store.writeMeta(r.meta))
}
