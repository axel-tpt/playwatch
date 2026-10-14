package resource

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/axel-tpt/playwatch/internal/ansible"
	"github.com/axel-tpt/playwatch/internal/ui"
)

// notPlaybooks are YAML files commonly found at a project root that are not
// playbooks.
var notPlaybooks = []string{"requirements.yml", "requirements.yaml", "galaxy.yml", "galaxy.yaml"}

// Playbooks lists the YAML files at the root of the project directory. Their
// content is never parsed: Enter asks ansible-playbook for an outline and r
// opens the run form.
func Playbooks(env Env) ui.Resource {
	// r opens the run form, prefilled with the options of the playbook's
	// last run (looked up off the UI loop).
	run := func(row ui.Row) tea.Cmd {
		return func() tea.Msg {
			var opts ansible.RunOptions
			history, _ := env.RunEnv().History()
			for _, info := range history {
				if info.Playbook == row.ID {
					opts = info.Options
					break
				}
			}
			return ui.Push(ui.NewRunForm(env.Ctx, env.RunEnv(), row.ID, opts))()
		}
	}
	return ui.Resource{
		Name:    "playbooks",
		Aliases: []string{"playbook", "pb"},
		Columns: []ui.Column{
			{Title: "name"},
			{Title: "size"},
			{Title: "modified"},
		},
		Load: func(context.Context) ([]ui.Row, error) {
			return listPlaybooks(env.Dir, env.PlaybookDir)
		},
		Enter: func(row ui.Row) tea.Cmd {
			return ui.Push(ui.NewTextView(env.Ctx, row.ID, func(ctx context.Context) (string, error) {
				return env.Client.PlaybookOutline(ctx, row.ID)
			}))
		},
		Actions: []ui.Action{
			{Key: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "run")), Run: run},
		},
	}
}

// DefaultPlaybookDir is where playbooks are looked up, relative to the
// project directory, unless overridden.
const DefaultPlaybookDir = "playbook"

// listPlaybooks lists the playbooks of playbookDir (relative to projectDir
// unless absolute). When playbookDir is empty the default directory is used,
// falling back to the project root if it does not exist. Row IDs are paths
// relative to projectDir (where ansible-playbook runs) when possible.
func listPlaybooks(projectDir, playbookDir string) ([]ui.Row, error) {
	if projectDir == "" {
		projectDir = "."
	}
	explicit := playbookDir != ""
	if !explicit {
		playbookDir = DefaultPlaybookDir
	}
	dir := playbookDir
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(projectDir, dir)
	}
	if !explicit {
		if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
			dir = projectDir
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("list playbooks: %w", err)
	}
	var rows []ui.Row
	for _, e := range entries {
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if e.IsDir() || (ext != ".yml" && ext != ".yaml") || slices.Contains(notPlaybooks, name) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		id := filepath.Join(dir, name)
		if rel, err := filepath.Rel(projectDir, id); err == nil && !strings.HasPrefix(rel, "..") {
			id = rel
		}
		rows = append(rows, ui.Row{ID: id, Cells: []string{
			name,
			fmt.Sprintf("%d B", info.Size()),
			info.ModTime().Format(time.DateTime),
		}})
	}
	return rows, nil
}
