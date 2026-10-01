package resource

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/axel-tpt/playwatch/internal/ui"
)

// Hosts lists inventory hosts, restricted to a group (and its descendants)
// when group is not empty.
func Hosts(env Env, group string) ui.Resource {
	describe := func(row ui.Row) tea.Cmd {
		return ui.Push(ui.NewTextView(env.Ctx, row.ID, func(ctx context.Context) (string, error) {
			return describeHost(ctx, env, row.ID)
		}))
	}
	return ui.Resource{
		Name:    "hosts",
		Aliases: []string{"host", "ho"},
		Columns: []ui.Column{
			{Title: "name"},
			{Title: "groups", MaxWidth: 40},
			{Title: "vars"},
			{Title: "ansible_host"},
		},
		Load: func(ctx context.Context) ([]ui.Row, error) {
			inv, err := env.Client.Inventory(ctx)
			if err != nil {
				return nil, err
			}
			names := inv.HostNames()
			if group != "" {
				names = inv.GroupHosts(group)
			}
			rows := make([]ui.Row, 0, len(names))
			for _, name := range names {
				h := inv.Hosts[name]
				rows = append(rows, ui.Row{ID: name, Cells: []string{
					name,
					strings.Join(h.Groups, ","),
					fmt.Sprint(len(h.Vars)),
					varString(h.Vars, "ansible_host"),
				}})
			}
			return rows, nil
		},
		Enter: describe,
		Actions: []ui.Action{
			{Key: key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "describe")), Run: describe},
		},
	}
}

// describeHost renders a host's groups, variables and session runs.
func describeHost(ctx context.Context, env Env, host string) (string, error) {
	inv, err := env.Client.Inventory(ctx)
	if err != nil {
		return "", err
	}
	h, ok := inv.Hosts[host]
	if !ok {
		return "", fmt.Errorf("host %q is not in the inventory", host)
	}
	vars, err := env.Client.HostVars(ctx, host)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	field := func(label, value string) {
		fmt.Fprintf(&b, "%s %s\n", ui.Muted(fmt.Sprintf("%-8s", label)), value)
	}
	field("Host", host)
	field("Groups", strings.Join(h.Groups, ", ")+ui.Muted("  (all: "+strings.Join(inv.HostGroups(host), ", ")+")"))

	b.WriteString("\n" + ui.Section("Runs") + "\n")
	var runs []string
	history, err := env.RunEnv().History()
	if err != nil {
		runs = append(runs, "  "+err.Error())
	}
	for _, info := range history {
		s, ok := info.Stats[host]
		if !ok {
			continue
		}
		runs = append(runs, fmt.Sprintf("  %s  %-8s %s  %s",
			info.Started.Format(time.DateTime), info.Playbook, runState(info), hostRecap(s)))
		if len(runs) == maxHostRuns {
			break
		}
	}
	if len(runs) == 0 {
		runs = []string{ui.Muted("  no recorded run on this host")}
	}
	b.WriteString(strings.Join(runs, "\n") + "\n")

	b.WriteString("\n" + ui.Section("Variables") + ui.Muted("  (ansible-inventory --host: inventory, group_vars and host_vars)") + "\n")
	b.WriteString(ui.ColorValue(vars))
	return b.String(), nil
}

// maxHostRuns is how many runs a host description lists.
const maxHostRuns = 10

func varString(vars map[string]any, name string) string {
	v, ok := vars[name]
	if !ok {
		return ""
	}
	return fmt.Sprint(v)
}

// sortedCopy returns a sorted copy of s.
func sortedCopy(s []string) []string {
	s = slices.Clone(s)
	slices.Sort(s)
	return s
}
