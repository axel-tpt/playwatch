package ui

import (
	"context"
	"strings"
	"time"

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

	// live, when set, makes the view reload itself until it is closed (the
	// source stopped changing), keeping the end of the text in view unless the
	// user scrolled up.
	live <-chan struct{}

	// colorToggle enables the keyColor binding, which strips the ANSI styles
	// from the content while plain is set.
	colorToggle bool
	plain       bool
}

var keyColor = key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "toggle colors"))

// WithColorToggle lets the user switch the content's ANSI colors on and off.
func (v *TextView) WithColorToggle() *TextView {
	v.colorToggle = true
	return v
}

type textTickMsg struct{ view *TextView }

func (m textTickMsg) Target() View { return m.view }

const textTickInterval = 500 * time.Millisecond

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

// NewLiveTextView is like NewTextView but follows a source that is still
// changing: the content is reloaded every half second until done is closed
// (plus one last time), and the view sticks to the bottom like `tail -f`.
func NewLiveTextView(ctx context.Context, title string, done <-chan struct{}, load func(ctx context.Context) (string, error)) *TextView {
	v := NewTextView(ctx, title, load)
	v.live = done
	return v
}

// StaticText returns a loader for content that is already known.
func StaticText(s string) func(context.Context) (string, error) {
	return func(context.Context) (string, error) { return s, nil }
}

func (v *TextView) Init() tea.Cmd {
	if v.live != nil {
		return tea.Batch(v.reload(), v.tick())
	}
	return v.reload()
}

func (v *TextView) tick() tea.Cmd {
	return tea.Tick(textTickInterval, func(time.Time) tea.Msg { return textTickMsg{view: v} })
}

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
	case textTickMsg:
		select {
		case <-v.live:
			return v.reload() // final reload, then stop ticking
		default:
		}
		if v.loading {
			return v.tick()
		}
		return tea.Batch(v.reload(), v.tick())
	case textLoadedMsg:
		v.loading = false
		follow := v.live != nil && v.vp.AtBottom()
		if msg.err != nil {
			v.content = styleError.Render(msg.err.Error())
		} else {
			v.content = msg.content
		}
		v.render()
		if follow {
			v.vp.GotoBottom()
		}
		return nil
	case tea.KeyPressMsg:
		if key.Matches(msg, keyRefresh) {
			return v.reload()
		}
		if v.colorToggle && key.Matches(msg, keyColor) {
			v.plain = !v.plain
			v.render()
			return nil
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
	if v.plain {
		content = ansi.Strip(content)
	}
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
	// The status line only shows the first few hints: put the specific keys first.
	var keys []key.Binding
	if v.colorToggle {
		keys = append(keys, keyColor)
	}
	return append(keys, keyRefresh, tableKeys.Up, tableKeys.Down, tableKeys.PageUp, tableKeys.PageDown, tableKeys.Top, tableKeys.Bottom)
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
