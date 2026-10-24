package resource

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/axel-tpt/playwatch/internal/ui"
)

// Roles lists the roles Ansible would find: the roles directories of the
// playbook directory and of the project, then the configured roles_path.
// Nothing inside a role is parsed.
func Roles(env Env) ui.Resource {
	project := env.Dir
	if project == "" {
		project = "."
	}
	// open shows the role's tasks file, or edits it in $EDITOR.
	open := func(row ui.Row, edit bool) tea.Cmd {
		file, err := roleTasksFile(row.ID)
		if err != nil {
			return ui.Error(err)
		}
		if edit {
			return ui.Edit(file)
		}
		title := filepath.Base(row.ID) + "/tasks/" + filepath.Base(file)
		return ui.Push(ui.NewTextView(env.Ctx, title, func(context.Context) (string, error) {
			data, err := os.ReadFile(file)
			if err != nil {
				return "", fmt.Errorf("read %s: %w", file, err)
			}
			return string(data), nil
		}))
	}
	return ui.Resource{
		Name:    "roles",
		Aliases: []string{"role", "ro"},
		Columns: []ui.Column{
			{Title: "name"},
			{Title: "tasks"},
			{Title: "path"},
		},
		Load: func(ctx context.Context) ([]ui.Row, error) {
			rolesPath, err := env.Client.RolesPath(ctx)
			if err != nil {
				return nil, err
			}
			dirs := roleSearchDirs(project, playbookDir(project, env.PlaybookDir), rolesPath)
			return listRoles(project, dirs)
		},
		Enter: func(row ui.Row) tea.Cmd { return open(row, false) },
		Actions: []ui.Action{
			{Key: key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit tasks")), Run: func(row ui.Row) tea.Cmd { return open(row, true) }},
		},
	}
}

// roleSearchDirs returns the directories holding roles, in Ansible's lookup
// order, without duplicates.
func roleSearchDirs(projectDir, pbDir string, rolesPath []string) []string {
	candidates := append([]string{filepath.Join(pbDir, "roles"), filepath.Join(projectDir, "roles")}, rolesPath...)
	var dirs []string
	for _, d := range candidates {
		if abs, err := filepath.Abs(d); err == nil {
			d = abs
		}
		if !slices.Contains(dirs, d) {
			dirs = append(dirs, d)
		}
	}
	return dirs
}

// listRoles lists the non-hidden subdirectories of dirs, in dirs order then by
// name. Missing directories are skipped. Row IDs are absolute role
// directories.
func listRoles(projectDir string, dirs []string) ([]ui.Row, error) {
	absProject, err := filepath.Abs(projectDir)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	var rows []ui.Row
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir) // sorted by name
		if err != nil {
			continue
		}
		shown := dir
		if rel, err := filepath.Rel(absProject, dir); err == nil && !strings.HasPrefix(rel, "..") {
			shown = rel
		}
		for _, e := range entries {
			name := e.Name()
			if strings.HasPrefix(name, ".") {
				continue
			}
			full := filepath.Join(dir, name)
			if fi, err := os.Stat(full); err != nil || !fi.IsDir() { // follows symlinks
				continue
			}
			tasks := "-"
			if f, err := roleTasksFile(full); err == nil {
				tasks = filepath.Base(f)
			}
			rows = append(rows, ui.Row{ID: full, Cells: []string{name, tasks, shown}})
		}
	}
	return rows, nil
}

// roleTasksFile returns the role's tasks/main.yml, or tasks/main.yaml.
func roleTasksFile(roleDir string) (string, error) {
	for _, name := range []string{"main.yml", "main.yaml"} {
		p := filepath.Join(roleDir, "tasks", name)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("role %s has no tasks/main.yml", filepath.Base(roleDir))
}
