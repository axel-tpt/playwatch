package ui

import (
	"context"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// TextView shows scrollable text produced by an asynchronous loader (a host
// description, a JSON result, command output…). The filter keeps matching
// lines only.
type TextView struct {
	ctx     context.Context
	title   string
	load    func(ctx context.Context) (string, error)
	content string
	filter  string
	loading bool
	vp      viewport.Model
}

type textLoadedMsg struct {
	view    *TextView
	content string
	err     error
}

func (m textLoadedMsg) Target() View { return m.view }

// NewTextView returns a view showing what load returns. Content may contain
// ANSI styles.
func NewTextView(ctx context.Context, title string, load func(ctx context.Context) (string, error)) *TextView {
	return &TextView{ctx: ctx, title: title, load: load, vp: viewport.New()}
}

// StaticText returns a loader for content that is already known.
func StaticText(s string) func(context.Context) (string, error) {
	return func(context.Context) (string, error) { return s, nil }
}

func (v *TextView) Init() tea.Cmd { return v.reload() }

func (v *TextView) reload() tea.Cmd {
	v.loading = true
	load, ctx := v.load, v.ctx
	return func() tea.Msg {
		s, err := load(ctx)
		return textLoadedMsg{view: v, content: s, err: err}
	}
}

func (v *TextView) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case textLoadedMsg:
		v.loading = false
		if msg.err != nil {
			v.content = styleError.Render(msg.err.Error())
		} else {
			v.content = msg.content
		}
		v.render()
		return nil
	case tea.KeyPressMsg:
		if key.Matches(msg, keyRefresh) {
			return v.reload()
		}
		if key.Matches(msg, tableKeys.Top) {
			v.vp.GotoTop()
			return nil
		}
		if key.Matches(msg, tableKeys.Bottom) {
			v.vp.GotoBottom()
			return nil
		}
	}
	var cmd tea.Cmd
	v.vp, cmd = v.vp.Update(msg)
	return cmd
}

func (v *TextView) render() {
	content := v.content
	if v.filter != "" {
		needle := strings.ToLower(v.filter)
		var kept []string
		for _, line := range strings.Split(content, "\n") {
			if strings.Contains(strings.ToLower(ansi.Strip(line)), needle) {
				kept = append(kept, line)
			}
		}
		content = strings.Join(kept, "\n")
	}
	v.vp.SetContent(content)
}

func (v *TextView) View() string {
	if v.loading && v.content == "" {
		return Muted(" loading…")
	}
	return v.vp.View()
}

func (v *TextView) Title() string { return v.title }

func (v *TextView) KeyMap() []key.Binding {
	return []key.Binding{tableKeys.Up, tableKeys.Down, tableKeys.PageUp, tableKeys.PageDown, tableKeys.Top, tableKeys.Bottom, keyRefresh}
}

func (v *TextView) SetSize(width, height int) {
	v.vp.SetWidth(width)
	v.vp.SetHeight(height)
}

func (v *TextView) SetFilter(f string) {
	v.filter = f
	v.render()
}

func (v *TextView) Filter() string { return v.filter }
