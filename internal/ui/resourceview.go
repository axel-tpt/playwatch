package ui

import (
	"context"
	"fmt"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// Resource declares a kind of object listed by a table view (hosts, groups,
// playbooks…). Registering a resource on the App makes it reachable with
// :<name>; nothing else in the UI needs to change.
type Resource struct {
	Name    string
	Aliases []string
	Columns []Column
	// Load fetches the rows. It runs outside the event loop.
	Load func(ctx context.Context) ([]Row, error)
	// Enter is the action bound to the enter key, if any.
	Enter func(Row) tea.Cmd
	// Actions are the resource-specific key bindings.
	Actions []Action
}

// Action is a key bound to an operation on the selected row.
type Action struct {
	Key key.Binding
	Run func(Row) tea.Cmd
}

var (
	keyEnter   = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open"))
	keyRefresh = key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("ctrl+r", "refresh"))
)

// ResourceView lists the rows of a Resource in a table.
type ResourceView struct {
	ctx     context.Context
	res     Resource
	title   string
	table   *Table
	loading bool
	err     error
}

type rowsLoadedMsg struct {
	view *ResourceView
	rows []Row
	err  error
}

func (m rowsLoadedMsg) Target() View { return m.view }

// NewResourceView returns a table view of res. The title defaults to the
// resource name.
func NewResourceView(ctx context.Context, res Resource, title string) *ResourceView {
	if title == "" {
		title = res.Name
	}
	return &ResourceView{ctx: ctx, res: res, title: title, table: NewTable(res.Columns...)}
}

func (v *ResourceView) Init() tea.Cmd { return v.load() }

func (v *ResourceView) load() tea.Cmd {
	v.loading = true
	load, ctx := v.res.Load, v.ctx
	return func() tea.Msg {
		rows, err := load(ctx)
		return rowsLoadedMsg{view: v, rows: rows, err: err}
	}
}

func (v *ResourceView) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case rowsLoadedMsg:
		v.loading = false
		v.err = msg.err
		if msg.err != nil {
			return Error(msg.err)
		}
		v.table.SetRows(msg.rows)
		return nil
	case tea.KeyPressMsg:
		if key.Matches(msg, keyRefresh) {
			return v.load()
		}
		row, ok := v.table.Selected()
		if key.Matches(msg, keyEnter) {
			if ok && v.res.Enter != nil {
				return v.res.Enter(row)
			}
			return nil
		}
		for _, a := range v.res.Actions {
			if key.Matches(msg, a.Key) {
				if !ok {
					return nil
				}
				return a.Run(row)
			}
		}
	}
	return v.table.Update(msg)
}

func (v *ResourceView) View() string {
	switch {
	case v.table.Len() > 0:
		return v.table.View()
	case v.loading:
		return Muted(" loading…")
	case v.err != nil:
		return styleError.Render(" " + v.err.Error())
	case v.table.Filter() != "":
		return Muted(" no match")
	}
	return Muted(" nothing to show")
}

func (v *ResourceView) Title() string {
	return fmt.Sprintf("%s(%d)", v.title, v.table.Len())
}

func (v *ResourceView) KeyMap() []key.Binding {
	keys := []key.Binding{}
	if v.res.Enter != nil {
		keys = append(keys, keyEnter)
	}
	for _, a := range v.res.Actions {
		keys = append(keys, a.Key)
	}
	keys = append(keys, keyRefresh)
	return append(keys, v.table.KeyMap()...)
}

func (v *ResourceView) SetSize(width, height int) { v.table.SetSize(width, height) }
func (v *ResourceView) SetFilter(f string)        { v.table.SetFilter(f) }
func (v *ResourceView) Filter() string            { return v.table.Filter() }
