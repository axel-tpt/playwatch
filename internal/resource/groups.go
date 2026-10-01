package resource

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/axel-tpt/playwatch/internal/ui"
)

// Groups lists inventory groups. Enter drills down to the group's hosts.
func Groups(env Env) ui.Resource {
	return ui.Resource{
		Name:    "groups",
		Aliases: []string{"group", "gr"},
		Columns: []ui.Column{
			{Title: "name"},
			{Title: "hosts"},
			{Title: "children", MaxWidth: 40},
			{Title: "vars"},
		},
		Load: func(ctx context.Context) ([]ui.Row, error) {
			inv, err := env.Client.Inventory(ctx)
			if err != nil {
				return nil, err
			}
			rows := make([]ui.Row, 0, len(inv.Groups))
			for _, name := range inv.GroupNames() {
				g := inv.Groups[name]
				rows = append(rows, ui.Row{ID: name, Cells: []string{
					name,
					fmt.Sprint(len(inv.GroupHosts(name))),
					strings.Join(sortedCopy(g.Children), ","),
					fmt.Sprint(len(g.Vars)),
				}})
			}
			return rows, nil
		},
		Enter: func(row ui.Row) tea.Cmd {
			return ui.Push(ui.NewResourceView(env.Ctx, Hosts(env, row.ID), "hosts@"+row.ID))
		},
	}
}
