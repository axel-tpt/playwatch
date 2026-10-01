package resource

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/axel-tpt/playwatch/internal/ansible"
	"github.com/axel-tpt/playwatch/internal/ui"
)

// Runs lists the runs of the project: those of this session and the stored
// history. Enter opens a run (live if it belongs to this session, replayed
// from the store otherwise).
func Runs(env Env) ui.Resource {
	// Load (off the UI loop) caches the runs it lists, so that actions find
	// them without any I/O.
	var (
		mu    sync.Mutex
		cache map[string]ui.RunInfo
	)
	find := func(id string) (ui.RunInfo, bool) {
		mu.Lock()
		defer mu.Unlock()
		info, ok := cache[id]
		return info, ok
	}
	withRun := func(f func(ui.RunInfo) tea.Cmd) func(ui.Row) tea.Cmd {
		return func(row ui.Row) tea.Cmd {
			info, ok := find(row.ID)
			if !ok {
				return ui.Error(fmt.Errorf("run %s not found", row.ID))
			}
			return f(info)
		}
	}
	rerun := func(onlyFailed bool) func(ui.RunInfo) tea.Cmd {
		return func(info ui.RunInfo) tea.Cmd {
			if info.State.Active() {
				return ui.Info("the run is still in progress")
			}
			return ui.Rerun(env.Ctx, env.RunEnv(), info, onlyFailed)
		}
	}

	return ui.Resource{
		Name:    "runs",
		Aliases: []string{"run"},
		Columns: []ui.Column{
			{Title: "started"},
			{Title: "playbook"},
			{Title: "state"},
			{Title: "duration"},
			{Title: "recap"},
			{Title: "args", MaxWidth: 40},
		},
		Load: func(context.Context) ([]ui.Row, error) {
			history, err := env.RunEnv().History()
			rows := make([]ui.Row, 0, len(history))
			infos := make(map[string]ui.RunInfo, len(history))
			for _, info := range history {
				infos[runRowID(info)] = info
				rows = append(rows, ui.Row{ID: runRowID(info), Cells: []string{
					info.Started.Format(time.DateTime),
					info.Playbook,
					runState(info),
					info.Duration.Round(100 * time.Millisecond).String(),
					recap(info.Stats),
					displayArgs(info.Options),
				}})
			}
			mu.Lock()
			cache = infos
			mu.Unlock()
			return rows, err
		},
		Enter: withRun(func(info ui.RunInfo) tea.Cmd {
			if info.View != nil {
				return ui.Push(info.View)
			}
			if env.Store == nil || info.StoreID == "" {
				return ui.Error(fmt.Errorf("run %s was not saved", info.Label()))
			}
			return ui.Push(ui.NewReplayView(env.Ctx, env.RunEnv(), info))
		}),
		Actions: []ui.Action{
			{Key: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "rerun")), Run: withRun(rerun(false))},
			{Key: key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "rerun on failed hosts")), Run: withRun(rerun(true))},
			{Key: key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "delete")), Run: withRun(func(info ui.RunInfo) tea.Cmd {
				if info.State.Active() {
					return ui.Info("the run is still in progress")
				}
				if env.Store == nil || info.StoreID == "" {
					return ui.Info("this run is not saved")
				}
				st, id := env.Store, info.StoreID
				return ui.Confirm(fmt.Sprintf("Delete run %s? [y/N]", info.Label()), func() tea.Msg {
					if err := st.Delete(id); err != nil {
						return ui.Error(err)()
					}
					return ui.Refresh()()
				})
			})},
		},
	}
}

// runRowID identifies a run in the table: its store ID when it was saved.
func runRowID(info ui.RunInfo) string {
	if info.StoreID != "" {
		return info.StoreID
	}
	return fmt.Sprintf("session-%d", info.ID)
}

func runState(info ui.RunInfo) string {
	s := string(info.State)
	switch info.State {
	case ui.RunOK:
		return ui.StatusStyle(ansible.StatusOK).Render(s)
	case ui.RunFailed, ui.RunInterrupted:
		return ui.StatusStyle(ansible.StatusFailed).Render(fmt.Sprintf("%s (rc=%d)", s, info.ExitCode))
	case ui.RunAborted:
		return ui.StatusStyle(ansible.StatusFailed).Render(s)
	}
	return ui.StatusStyle(ansible.StatusRunning).Render(s)
}

func recap(stats map[string]ansible.HostStats) string {
	if stats == nil {
		return ""
	}
	var total ansible.HostStats
	for _, s := range stats {
		total.OK += s.OK
		total.Changed += s.Changed
		total.Failures += s.Failures
		total.Unreachable += s.Unreachable
	}
	return hostRecap(total)
}

// hostRecap renders ok/changed counts, and failures only when there are some.
func hostRecap(s ansible.HostStats) string {
	parts := []string{
		ui.StatusStyle(ansible.StatusOK).Render(fmt.Sprintf("ok=%d", s.OK)),
		ui.StatusStyle(ansible.StatusChanged).Render(fmt.Sprintf("changed=%d", s.Changed)),
	}
	if s.Failures > 0 {
		parts = append(parts, ui.StatusStyle(ansible.StatusFailed).Render(fmt.Sprintf("failed=%d", s.Failures)))
	}
	if s.Unreachable > 0 {
		parts = append(parts, ui.StatusStyle(ansible.StatusUnreachable).Render(fmt.Sprintf("unreachable=%d", s.Unreachable)))
	}
	return strings.Join(parts, " ")
}

// displayArgs shows a run's options as ansible-playbook arguments. A typed-in
// vault password shows as the equivalent --ask-vault-pass.
func displayArgs(o ansible.RunOptions) string {
	args := o.Args()
	if o.AskVaultPass {
		args = append(args, "--ask-vault-pass")
	}
	return strings.Join(args, " ")
}
