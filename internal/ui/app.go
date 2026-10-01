package ui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/axel-tpt/playwatch/internal/ansible"
)

var (
	keyCommand = key.NewBinding(key.WithKeys(":"), key.WithHelp(":", "command"))
	keyFilter  = key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter"))
	keyHelp    = key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help"))
	keyBack    = key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back / clear filter"))
	keyQuit    = key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit"))
)

type promptMode int

const (
	promptNone promptMode = iota
	promptCommand
	promptFilter
)

// Config configures the App.
type Config struct {
	// Context lines shown in the header (e.g. "inventory" → "hosts.ini").
	Context [][2]string
	// Resources reachable with :<name>.
	Resources []Resource
	// Start is the resource shown first.
	Start string
	// Runs is the session's run list, used to confirm quitting while runs
	// are in progress.
	Runs *RunList
}

// App is the root model: a stack of views, a command bar, a filter prompt and
// a status bar.
type App struct {
	ctx       context.Context
	cfg       Config
	resources map[string]Resource
	names     []string

	stack []View
	live  map[View]bool

	width, height int
	mode          promptMode
	prompt        textinput.Model
	confirm       *confirmMsg
	status        string
	statusErr     bool
}

// NewApp returns the root model.
func NewApp(ctx context.Context, cfg Config) *App {
	a := &App{ctx: ctx, cfg: cfg, resources: map[string]Resource{}, live: map[View]bool{}}
	for _, r := range cfg.Resources {
		a.names = append(a.names, r.Name)
		a.resources[r.Name] = r
		for _, alias := range r.Aliases {
			a.resources[alias] = r
		}
	}
	a.prompt = textinput.New()
	a.prompt.Prompt = ""
	a.prompt.ShowSuggestions = true
	a.prompt.SetSuggestions(append(slices.Clone(a.names), "quit"))
	return a
}

// Run starts the TUI and blocks until the user quits. Runs still in progress
// are interrupted and waited for.
func Run(ctx context.Context, cfg Config) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	_, err := tea.NewProgram(NewApp(ctx, cfg), tea.WithContext(ctx)).Run()
	cancel()
	if cfg.Runs != nil {
		cfg.Runs.Wait(15 * time.Second)
	}
	if err != nil && ctx.Err() == nil {
		return fmt.Errorf("run ui: %w", err)
	}
	return nil
}

func (a *App) Init() tea.Cmd {
	if _, ok := a.resources[a.cfg.Start]; !ok {
		return Error(fmt.Errorf("unknown resource %q", a.cfg.Start))
	}
	return a.switchTo(a.cfg.Start)
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd := a.update(msg)
	a.layout()
	return a, cmd
}

func (a *App) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = msg.Width, msg.Height
		return nil
	case pushMsg:
		return a.push(msg.view)
	case replaceMsg:
		if len(a.stack) > 0 {
			a.stack = a.stack[:len(a.stack)-1]
		}
		return a.push(msg.view)
	case popMsg:
		a.pop()
		return nil
	case switchMsg:
		return a.switchTo(msg.resource)
	case statusMsg:
		a.status, a.statusErr = msg.text, msg.err
		return nil
	case confirmMsg:
		a.confirm = &msg
		return nil
	case keepAliveMsg:
		a.live[msg.view] = true
		return nil
	case refreshMsg:
		if rv, ok := a.top().(*ResourceView); ok {
			return rv.load()
		}
		return nil
	case Targeted:
		if t := msg.Target(); a.live[t] || slices.Contains(a.stack, t) {
			return t.Update(msg)
		}
		return nil
	case tea.KeyPressMsg:
		return a.handleKey(msg)
	}
	if a.mode != promptNone {
		var cmd tea.Cmd
		a.prompt, cmd = a.prompt.Update(msg)
		return cmd
	}
	if top := a.top(); top != nil {
		return top.Update(msg)
	}
	return nil
}

func (a *App) handleKey(k tea.KeyPressMsg) tea.Cmd {
	if a.confirm != nil {
		c := a.confirm
		a.confirm = nil
		if k.String() == "y" || k.String() == "Y" {
			return c.onYes
		}
		return nil
	}
	if a.mode != promptNone {
		return a.handlePromptKey(k)
	}
	a.status, a.statusErr = "", false

	top := a.top()
	if c, ok := top.(InputCapturer); ok && c.CapturesInput() && !key.Matches(k, keyQuit) {
		return top.Update(k)
	}
	switch {
	case key.Matches(k, keyQuit):
		if i, ok := top.(Interrupter); ok {
			if cmd, handled := i.Interrupt(); handled {
				return cmd
			}
		}
		return a.quit()
	case key.Matches(k, keyCommand):
		return a.openPrompt(promptCommand, "")
	case key.Matches(k, keyFilter):
		if f, ok := top.(Filterable); ok {
			return a.openPrompt(promptFilter, f.Filter())
		}
		return nil
	case key.Matches(k, keyHelp):
		return a.push(NewTextView(a.ctx, "help", StaticText(a.helpText())))
	case key.Matches(k, keyBack):
		if f, ok := top.(Filterable); ok && f.Filter() != "" {
			f.SetFilter("")
			return nil
		}
		a.pop()
		return nil
	}
	if top != nil {
		return top.Update(k)
	}
	return nil
}

func (a *App) handlePromptKey(k tea.KeyPressMsg) tea.Cmd {
	switch k.String() {
	case "esc":
		if f, ok := a.top().(Filterable); ok && a.mode == promptFilter {
			f.SetFilter("")
		}
		a.closePrompt()
		return nil
	case "enter":
		value := strings.TrimSpace(a.prompt.Value())
		mode := a.mode
		a.closePrompt()
		if mode == promptCommand {
			return a.runCommand(value)
		}
		return nil
	}
	var cmd tea.Cmd
	a.prompt, cmd = a.prompt.Update(k)
	if f, ok := a.top().(Filterable); ok && a.mode == promptFilter {
		f.SetFilter(a.prompt.Value())
	}
	return cmd
}

func (a *App) openPrompt(mode promptMode, value string) tea.Cmd {
	a.mode = mode
	a.prompt.ShowSuggestions = mode == promptCommand
	a.prompt.SetValue(value)
	a.prompt.CursorEnd()
	return a.prompt.Focus()
}

func (a *App) closePrompt() {
	a.mode = promptNone
	a.prompt.Blur()
	a.prompt.SetValue("")
}

func (a *App) runCommand(cmd string) tea.Cmd {
	switch cmd {
	case "":
		return nil
	case "q", "q!", "quit":
		return a.quit()
	}
	if _, ok := a.resources[cmd]; !ok {
		return Error(fmt.Errorf("unknown command %q (try :%s)", cmd, strings.Join(a.names, ", :")))
	}
	return a.switchTo(cmd)
}

func (a *App) quit() tea.Cmd {
	if a.cfg.Runs != nil {
		if n := a.cfg.Runs.Active(); n > 0 {
			return Confirm(fmt.Sprintf("%d run(s) in progress: interrupt them and quit? [y/N]", n), tea.Quit)
		}
	}
	return tea.Quit
}

func (a *App) switchTo(name string) tea.Cmd {
	res := a.resources[name]
	a.stack = nil
	return a.push(NewResourceView(a.ctx, res, ""))
}

func (a *App) push(v View) tea.Cmd {
	a.stack = append(a.stack, v)
	a.layout()
	return v.Init()
}

func (a *App) pop() {
	if len(a.stack) > 1 {
		a.stack = a.stack[:len(a.stack)-1]
	}
}

func (a *App) top() View {
	if len(a.stack) == 0 {
		return nil
	}
	return a.stack[len(a.stack)-1]
}

// Layout: header, optional prompt line, framed body, status bar.
func (a *App) bodySize() (int, int) {
	h := a.height - 4 // header, status bar, top and bottom borders
	if a.mode != promptNone || a.confirm != nil {
		h--
	}
	return max(a.width-2, 0), max(h, 0)
}

func (a *App) layout() {
	if top := a.top(); top != nil {
		top.SetSize(a.bodySize())
	}
}

func (a *App) View() tea.View {
	v := tea.NewView(a.render())
	v.AltScreen = true
	v.WindowTitle = "playwatch"
	return v
}

func (a *App) render() string {
	if a.width <= 0 || a.height <= 0 {
		return ""
	}
	lines := []string{a.headerLine()}
	switch {
	case a.confirm != nil:
		lines = append(lines, " "+styleConfirm.Render(a.confirm.prompt))
	case a.mode == promptCommand:
		lines = append(lines, " "+stylePrompt.Render(":")+a.prompt.View())
	case a.mode == promptFilter:
		lines = append(lines, " "+stylePrompt.Render("/")+a.prompt.View())
	}
	lines = append(lines, a.frame()...)
	lines = append(lines, a.statusLine())
	return strings.Join(lines, "\n")
}

func (a *App) headerLine() string {
	parts := []string{styleHeader.Render(" playwatch")}
	for _, kv := range a.cfg.Context {
		parts = append(parts, styleHeaderLabel.Render(kv[0]+":")+" "+kv[1])
	}
	if a.cfg.Runs != nil {
		if n := a.cfg.Runs.Active(); n > 0 {
			parts = append(parts, StatusStyle(ansible.StatusRunning).Render(fmt.Sprintf("%d run(s) in progress", n)))
		}
	}
	return fit(strings.Join(parts, "  "), a.width)
}

func (a *App) frame() []string {
	w, h := a.bodySize()
	top := a.top()
	title := ""
	if top != nil {
		crumbs := make([]string, len(a.stack))
		for i, v := range a.stack {
			crumbs[i] = v.Title()
		}
		title = styleTitle.Render(strings.Join(crumbs, " › "))
		if f, ok := top.(Filterable); ok && f.Filter() != "" {
			title += styleTitleCount.Render(" </" + f.Filter() + ">")
		}
	}
	title = ansi.Truncate(title, max(w-4, 0), "…")
	fill := max(w-3-ansi.StringWidth(title), 0)
	lines := []string{styleBorder.Render("┌─ ") + title + styleBorder.Render(" "+strings.Repeat("─", fill)+"┐")}

	var body []string
	if top != nil {
		body = strings.Split(top.View(), "\n")
	}
	side := styleBorder.Render("│")
	for i := range h {
		line := ""
		if i < len(body) {
			line = body[i]
		}
		lines = append(lines, side+fit(line, w)+side)
	}
	return append(lines, styleBorder.Render("└"+strings.Repeat("─", w)+"┘"))
}

func (a *App) statusLine() string {
	if a.status != "" {
		style := styleInfo
		if a.statusErr {
			style = styleError
		}
		return fit(style.Render(" "+firstLine(a.status)), a.width)
	}
	var hints []string
	if top := a.top(); top != nil {
		for _, b := range top.KeyMap() {
			if len(hints) == 4 {
				break
			}
			hints = append(hints, hint(b))
		}
	}
	hints = append(hints, hint(keyCommand), hint(keyFilter), hint(keyHelp))
	return fit(" "+strings.Join(hints, "  "), a.width)
}

func hint(b key.Binding) string {
	h := b.Help()
	return styleKey.Render("<"+h.Key+">") + " " + Muted(h.Desc)
}

func (a *App) helpText() string {
	var b strings.Builder
	section := func(title string, bindings []key.Binding) {
		b.WriteString(styleSection.Render(title) + "\n")
		for _, k := range bindings {
			h := k.Help()
			fmt.Fprintf(&b, "  %s %s\n", styleKey.Render(fmt.Sprintf("%-10s", h.Key)), h.Desc)
		}
		b.WriteString("\n")
	}
	if top := a.top(); top != nil {
		section(top.Title(), top.KeyMap())
	}
	section("General", []key.Binding{keyCommand, keyFilter, keyBack, keyHelp, keyQuit})
	b.WriteString(styleSection.Render("Commands") + "\n")
	for _, name := range a.names {
		r := a.resources[name]
		aliases := ""
		if len(r.Aliases) > 0 {
			aliases = Muted(" (" + strings.Join(r.Aliases, ", ") + ")")
		}
		fmt.Fprintf(&b, "  %s%s\n", styleKey.Render(":"+name), aliases)
	}
	fmt.Fprintf(&b, "  %s\n", styleKey.Render(":quit"))
	return b.String()
}

// fit pads or truncates a styled line to exactly width cells.
func fit(s string, width int) string {
	s = ansi.Truncate(s, width, "")
	if pad := width - ansi.StringWidth(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}
