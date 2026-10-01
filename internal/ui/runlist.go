package ui

import (
	"slices"
	"sync"
	"time"

	"github.com/axel-tpt/playwatch/internal/ansible"
	"github.com/axel-tpt/playwatch/internal/store"
)

// RunState is the lifecycle state of a run. Final states share their names
// with the store's.
type RunState string

const (
	RunStarting    RunState = "starting"
	RunRunning     RunState = store.StateRunning
	RunOK          RunState = store.StateOK
	RunFailed      RunState = store.StateFailed
	RunInterrupted RunState = store.StateInterrupted
	RunAborted     RunState = store.StateAborted
)

// Active reports whether the run is still in progress.
func (s RunState) Active() bool { return s == RunStarting || s == RunRunning }

// RunInfo is a snapshot of a run, safe to read from any goroutine.
type RunInfo struct {
	ID          int    // session number, 0 for runs of previous sessions
	StoreID     string // empty when the run is not persisted
	Playbook    string
	Inventories []string
	Options     ansible.RunOptions
	State       RunState
	ExitCode    int
	Started     time.Time
	Duration    time.Duration
	Stats       map[string]ansible.HostStats
	// View is the live view of a run of this session, nil otherwise.
	View *RunView
}

// Label is a short human name for the run.
func (i RunInfo) Label() string {
	return i.Playbook + "@" + i.Started.Format("15:04:05")
}

func infoFromMeta(m store.Meta) RunInfo {
	return RunInfo{
		StoreID:     m.ID,
		Playbook:    m.Playbook,
		Inventories: m.Inventories,
		Options:     m.RunOptions(),
		State:       RunState(m.State),
		ExitCode:    m.ExitCode,
		Started:     m.Started,
		Duration:    m.Duration(),
		Stats:       m.Stats,
	}
}

// RunEnv is what run views need: where to run playbooks and where to keep
// their history.
type RunEnv struct {
	Client *ansible.Client
	Runs   *RunList
	// Store persists runs; nil disables persistence.
	Store *store.Store
	// Project is the absolute project directory, used to scope the history.
	Project string
}

// History returns the runs of this session and the stored runs of the
// project, most recent first. A run of this session shows its live state.
func (env RunEnv) History() ([]RunInfo, error) {
	session := env.Runs.Infos()
	infos := slices.Clone(session)
	var err error
	if env.Store != nil {
		var metas []store.Meta
		metas, err = env.Store.List(env.Project)
		for _, m := range metas {
			if !slices.ContainsFunc(session, func(i RunInfo) bool { return i.StoreID == m.ID }) {
				infos = append(infos, infoFromMeta(m))
			}
		}
	}
	slices.SortFunc(infos, func(a, b RunInfo) int { return b.Started.Compare(a.Started) })
	return infos, err
}

// RunList holds the runs of the session. Runs keep going when their view is
// popped; the list lets the user get back to them.
type RunList struct {
	mu    sync.Mutex
	next  int
	infos map[int]RunInfo
	views []*RunView
}

// NewRunList returns an empty list.
func NewRunList() *RunList { return &RunList{infos: map[int]RunInfo{}} }

// Infos returns a snapshot of the session's runs, most recent first.
func (l *RunList) Infos() []RunInfo {
	l.mu.Lock()
	defer l.mu.Unlock()
	infos := make([]RunInfo, 0, len(l.infos))
	for _, info := range l.infos {
		infos = append(infos, info)
	}
	slices.SortFunc(infos, func(a, b RunInfo) int { return b.ID - a.ID })
	return infos
}

// Active returns the number of runs still in progress.
func (l *RunList) Active() int {
	n := 0
	for _, info := range l.Infos() {
		if info.State.Active() {
			n++
		}
	}
	return n
}

// Wait waits for all runs to be over and recorded, at most timeout.
func (l *RunList) Wait(timeout time.Duration) {
	l.mu.Lock()
	views := slices.Clone(l.views)
	l.mu.Unlock()
	deadline := time.After(timeout)
	for _, v := range views {
		select {
		case <-v.done:
		case <-deadline:
			return
		}
	}
}

func (l *RunList) register(v *RunView) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.next++
	l.views = append(l.views, v)
	return l.next
}

func (l *RunList) publish(info RunInfo) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.infos[info.ID] = info
}
