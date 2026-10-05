package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/axel-tpt/playwatch/internal/ansible"
	"github.com/axel-tpt/playwatch/internal/store"
)

var (
	keyRunOutput = key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "raw output"))
	keyRunResult = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "result (task column: all hosts)"))
	keyRunCancel = key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "interrupt run"))
	keyRerun     = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "rerun"))
	keyRerunFail = key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "rerun on failed hosts"))
)

// RunView shows a task × host matrix: live for a run of this session, or
// replayed from the store for a past run.
type RunView struct {
	parent context.Context // outlives the run: used for views opened from this one
	ctx    context.Context // the run itself, cancelled once it is over
	cancel context.CancelFunc
	env    RunEnv
	info   RunInfo
	replay bool // past run loaded from the store

	output *outputBuffer // live runs only
	err    error

	vaultPassword []byte // cleared once the run has started

	// Filled by the pump goroutine, drained by the UI.
	mu      sync.Mutex
	pending []ansible.Event
	end     *runEnd
	notify  chan struct{}
	done    chan struct{} // closed once the run is over and recorded

	hosts  []string
	tasks  []string // row IDs in order: "play:<uuid>" or "task:<uuid>"
	labels map[string]string
	cells  map[string]map[string]*cell // task UUID → host → cell
	width  int
	table  *Table
}

type cell struct {
	status  ansible.Status
	items   int
	last    ansible.Event
	aborted bool // still running when the run ended
}

type runEnd struct {
	state    RunState
	exitCode int
	err      error // ansible-playbook's error
	recErr   error // persistence error
}

type (
	runStartedMsg struct {
		view    *RunView
		storeID string
		err     error
		recErr  error
	}
	runUpdateMsg struct {
		view   *RunView
		events []ansible.Event
		end    *runEnd
	}
	runTickMsg   struct{ view *RunView }
	replayLoaded struct {
		view   *RunView
		events []ansible.Event
		err    error
	}
)

func (m runStartedMsg) Target() View { return m.view }
func (m runUpdateMsg) Target() View  { return m.view }
func (m runTickMsg) Target() View    { return m.view }
func (m replayLoaded) Target() View  { return m.view }

func newRunView(ctx context.Context, env RunEnv, info RunInfo) *RunView {
	parent := ctx
	ctx, cancel := context.WithCancel(parent)
	v := &RunView{
		parent: parent,
		ctx:    ctx,
		cancel: cancel,
		env:    env,
		info:   info,
		labels: map[string]string{},
		cells:  map[string]map[string]*cell{},
		table:  NewTable(Column{Title: "task", MaxWidth: 50}),
	}
	v.table.EnableCellCursor()
	return v
}

// NewRunView prepares a run of playbook. vaultPassword, if any, is handed to
// Ansible through a pipe and forgotten once the run has started. The run
// starts when the view is initialized and keeps going if the view is popped.
func NewRunView(ctx context.Context, env RunEnv, playbook string, opts ansible.RunOptions, vaultPassword []byte) *RunView {
	opts.AskVaultPass = len(vaultPassword) > 0
	v := newRunView(ctx, env, RunInfo{
		Playbook:    playbook,
		Inventories: env.Client.Inventories,
		Options:     opts,
		State:       RunStarting,
		ExitCode:    -1,
	})
	v.vaultPassword = vaultPassword
	v.info.View = v
	v.output = &outputBuffer{max: 1 << 20}
	v.notify = make(chan struct{}, 1)
	v.done = make(chan struct{})
	v.info.ID = env.Runs.register(v)
	return v
}

// NewReplayView shows a stored run.
func NewReplayView(ctx context.Context, env RunEnv, info RunInfo) *RunView {
	v := newRunView(ctx, env, info)
	v.replay = true
	v.info.View = nil
	return v
}

func (v *RunView) Init() tea.Cmd {
	if v.replay {
		return v.loadReplay()
	}
	if v.info.State != RunStarting || !v.info.Started.IsZero() {
		return nil // already started: the view is being shown again
	}
	v.info.Started = time.Now()
	v.publish()
	return tea.Batch(KeepAlive(v), v.start())
}

func (v *RunView) start() tea.Cmd {
	info, env, ctx, buf := v.info, v.env, v.ctx, v.output
	password := v.vaultPassword
	v.vaultPassword = nil
	return func() tea.Msg {
		defer clear(password)
		var rec *store.Recorder
		var recErr error
		out := io.Writer(buf)
		if env.Store != nil {
			rec, recErr = env.Store.Create(store.Meta{
				Playbook:    info.Playbook,
				Project:     env.Project,
				Inventories: info.Inventories,
				Options:     &info.Options,
				Args:        info.Options.Args(),
				Started:     info.Started,
			})
			if rec != nil {
				out = io.MultiWriter(buf, rec.Output())
			}
		}
		run, err := env.Client.RunPlaybook(ctx, ansible.PlaybookOptions{
			Playbook:      info.Playbook,
			Args:          info.Options.Args(),
			Output:        out,
			VaultPassword: password,
			Color:         true, // shown in the raw output view, which renders ANSI
		})
		msg := runStartedMsg{view: v, err: err, recErr: recErr}
		if rec != nil {
			msg.storeID = rec.Meta().ID
		}
		if err != nil {
			if rec != nil {
				_, _ = io.WriteString(rec.Output(), err.Error())
				_ = rec.Finish(store.StateFailed, -1, nil)
			}
			close(v.done)
			return msg
		}
		go v.pump(run, rec)
		return msg
	}
}

// pump drains the run's events, records them and hands them to the UI. It
// never waits for the UI, so a run never blocks on it (even after quitting).
func (v *RunView) pump(run *ansible.Run, rec *store.Recorder) {
	defer close(v.done)
	var stats map[string]ansible.HostStats
	var recErr error
	for e := range run.Events {
		if rec != nil && recErr == nil {
			recErr = rec.Record(e)
		}
		if e.Type == ansible.EventStats {
			stats = e.Stats
		}
		v.mu.Lock()
		v.pending = append(v.pending, e)
		v.mu.Unlock()
		v.signal()
	}
	err := run.Wait()
	end := &runEnd{state: finalState(v.ctx.Err(), err), exitCode: run.ExitCode(), err: err, recErr: recErr}
	if rec != nil {
		if ferr := rec.Finish(string(end.state), end.exitCode, stats); end.recErr == nil {
			end.recErr = ferr
		}
	}
	v.mu.Lock()
	v.end = end
	v.mu.Unlock()
	v.signal()
}

func (v *RunView) signal() {
	select {
	case v.notify <- struct{}{}:
	default:
	}
}

// wait returns the events received since the last call, and the end of the
// run once it is over.
func (v *RunView) wait() tea.Cmd {
	return func() tea.Msg {
		<-v.notify
		v.mu.Lock()
		defer v.mu.Unlock()
		msg := runUpdateMsg{view: v, events: v.pending, end: v.end}
		v.pending = nil
		return msg
	}
}

func finalState(ctxErr, err error) RunState {
	switch {
	case ctxErr != nil:
		return RunInterrupted
	case err != nil:
		return RunFailed
	}
	return RunOK
}

// tick refreshes the elapsed time while the run is in progress.
func (v *RunView) tick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return runTickMsg{view: v} })
}

func (v *RunView) loadReplay() tea.Cmd {
	st, id := v.env.Store, v.info.StoreID
	return func() tea.Msg {
		events, err := st.Events(id)
		return replayLoaded{view: v, events: events, err: err}
	}
}

func (v *RunView) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case runStartedMsg:
		v.info.StoreID = msg.storeID
		var cmds []tea.Cmd
		if msg.recErr != nil {
			cmds = append(cmds, Error(fmt.Errorf("run not saved: %w", msg.recErr)))
		}
		if msg.err != nil {
			v.finish(&runEnd{state: RunFailed, exitCode: -1, err: msg.err})
			return tea.Batch(append(cmds, Error(msg.err))...)
		}
		v.info.State = RunRunning
		v.publish()
		return tea.Batch(append(cmds, v.wait(), v.tick())...)
	case runTickMsg:
		if v.info.State != RunRunning {
			return nil
		}
		v.publish()
		return v.tick()
	case runUpdateMsg:
		for _, e := range msg.events {
			v.apply(e)
		}
		if msg.end == nil {
			v.publish()
			return v.wait()
		}
		v.finish(msg.end)
		if msg.end.recErr != nil {
			return Error(fmt.Errorf("run %s not fully saved: %w", v.info.Label(), msg.end.recErr))
		}
		if v.info.State == RunFailed {
			return Error(fmt.Errorf("run %s: exited with code %d", v.info.Label(), v.info.ExitCode))
		}
		return Info(fmt.Sprintf("run %s: %s", v.info.Label(), v.info.State))
	case replayLoaded:
		if msg.err != nil {
			v.err = msg.err
			return Error(msg.err)
		}
		for _, e := range msg.events {
			v.apply(e)
		}
		if !v.info.State.Active() {
			v.markAborted()
		}
		return nil
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, keyRunResult):
			return v.openResult()
		case key.Matches(msg, keyRunOutput):
			return Push(v.outputView())
		case key.Matches(msg, keyRerun):
			if v.info.State.Active() {
				return Info("the run is still in progress")
			}
			return Rerun(v.parent, v.env, v.info, false)
		case key.Matches(msg, keyRerunFail):
			if v.info.State.Active() {
				return Info("the run is still in progress")
			}
			return Rerun(v.parent, v.env, v.info, true)
		}
	}
	return v.table.Update(msg)
}

func (v *RunView) outputView() *TextView {
	title := "output " + v.info.Label()
	if v.replay {
		st, id := v.env.Store, v.info.StoreID
		return NewTextView(v.parent, title, func(context.Context) (string, error) { return st.Output(id) }).WithColorToggle()
	}
	out := v.output
	// v.ctx is cancelled once the run is over: stop following then.
	return NewLiveTextView(v.parent, title, v.ctx.Done(), func(context.Context) (string, error) { return out.String(), nil }).WithColorToggle()
}

// Rerun opens the run form prefilled with info's playbook, inventories and
// options. With onlyFailed, the run is limited to the hosts that failed or
// were unreachable.
func Rerun(ctx context.Context, env RunEnv, info RunInfo, onlyFailed bool) tea.Cmd {
	opts := info.Options
	if onlyFailed {
		hosts := make([]string, 0, len(info.Stats))
		for h := range info.Stats {
			hosts = append(hosts, h)
		}
		slices.Sort(hosts)
		failed := ansible.FailedHosts(info.Stats, hosts)
		if len(failed) == 0 {
			return Info("no failed or unreachable host in this run")
		}
		opts = opts.WithLimit(strings.Join(failed, ","))
	}
	if len(info.Inventories) > 0 {
		client := *env.Client
		client.Inventories = info.Inventories
		env.Client = &client
	}
	return Push(NewRunForm(ctx, env, info.Playbook, opts))
}

// Interrupt cancels the run while it is in progress.
func (v *RunView) Interrupt() (tea.Cmd, bool) {
	if v.replay || !v.info.State.Active() {
		return nil, false
	}
	v.cancel()
	return Info(fmt.Sprintf("run %s: interrupting…", v.info.Label())), true
}

func (v *RunView) finish(end *runEnd) {
	v.err = end.err
	v.info.State = end.state
	v.info.ExitCode = end.exitCode
	v.info.Duration = time.Since(v.info.Started)
	v.cancel() // release the context
	v.markAborted()
	v.publish()
}

// markAborted flags the cells left running when the run ended.
func (v *RunView) markAborted() {
	for _, byHost := range v.cells {
		for _, c := range byHost {
			if c.status == ansible.StatusRunning {
				c.aborted = true
			}
		}
	}
	v.refreshTable()
}

func (v *RunView) publish() {
	if v.replay {
		return
	}
	info := v.info
	if info.State.Active() {
		info.Duration = time.Since(info.Started)
	}
	v.env.Runs.publish(info)
}

// apply folds an event into the matrix.
func (v *RunView) apply(e ansible.Event) {
	switch e.Type {
	case ansible.EventPlayStart:
		id := "play:" + e.PlayUUID
		v.tasks = append(v.tasks, id)
		v.labels[id] = styleSection.Render("▸ " + e.Play)
	case ansible.EventTaskStart:
		id := "task:" + e.TaskUUID
		if _, seen := v.labels[id]; !seen {
			v.tasks = append(v.tasks, id)
		}
		label := "  " + e.Task
		if e.IsHandler {
			label = "  " + Muted("handler:") + " " + e.Task
		}
		v.labels[id] = label
	case ansible.EventStats:
		v.info.Stats = e.Stats
	default:
		if e.Host == "" || e.TaskUUID == "" {
			return
		}
		if !slices.Contains(v.hosts, e.Host) {
			v.hosts = append(v.hosts, e.Host)
		}
		byHost := v.cells[e.TaskUUID]
		if byHost == nil {
			byHost = map[string]*cell{}
			v.cells[e.TaskUUID] = byHost
		}
		c := byHost[e.Host]
		if c == nil {
			c = &cell{}
			byHost[e.Host] = c
		}
		switch e.Type {
		case ansible.EventItemOK, ansible.EventItemFailed, ansible.EventItemSkipped:
			c.items++
		default:
			c.status = e.Status()
			c.last = e
		}
	}
	v.refreshTable()
}

func (v *RunView) refreshTable() {
	cols := []Column{{Title: "task", MaxWidth: 50}}
	for _, h := range v.hosts {
		cols = append(cols, Column{Title: h})
	}
	v.table.SetColumns(cols...)

	rows := make([]Row, 0, len(v.tasks))
	for _, id := range v.tasks {
		cells := []string{v.labels[id]}
		if uuid, ok := strings.CutPrefix(id, "task:"); ok {
			for _, h := range v.hosts {
				cells = append(cells, renderCell(v.cells[uuid][h]))
			}
		}
		rows = append(rows, Row{ID: id, Cells: cells})
	}
	v.table.SetRows(rows)
}

func renderCell(c *cell) string {
	if c == nil || c.status == ansible.StatusNone {
		return ""
	}
	if c.aborted {
		return Muted("aborted")
	}
	text := string(c.status)
	switch c.status {
	case ansible.StatusRunning:
		text = "…"
	case ansible.StatusUnreachable:
		text = "unreach"
	}
	if c.items > 0 && c.status != ansible.StatusRunning {
		text += fmt.Sprintf(" ×%d", c.items)
	}
	return StatusStyle(c.status).Render(text)
}

func (v *RunView) View() string {
	return v.summary() + "\n" + v.table.View()
}

// summary is the line above the matrix: state, duration and recap.
func (v *RunView) summary() string {
	info := v.info
	dur := info.Duration
	if info.State == RunRunning {
		dur = time.Since(info.Started)
	}
	state := string(info.State)
	switch info.State {
	case RunOK:
		state = StatusStyle(ansible.StatusOK).Render(state)
	case RunFailed, RunInterrupted:
		state = StatusStyle(ansible.StatusFailed).Render(fmt.Sprintf("%s (rc=%d)", state, info.ExitCode))
	case RunAborted:
		state = StatusStyle(ansible.StatusFailed).Render(state)
	default:
		state = StatusStyle(ansible.StatusRunning).Render(state)
	}
	parts := []string{
		styleHeaderLabel.Render("playbook ") + info.Playbook,
		styleHeaderLabel.Render("state ") + state,
		styleHeaderLabel.Render("time ") + dur.Round(100*time.Millisecond).String(),
	}
	if info.Stats != nil {
		var ok, changed, failed, unreachable int
		for _, s := range info.Stats {
			ok += s.OK
			changed += s.Changed
			failed += s.Failures
			unreachable += s.Unreachable
		}
		parts = append(parts, fmt.Sprintf("%s %s %s %s",
			StatusStyle(ansible.StatusOK).Render(fmt.Sprintf("ok=%d", ok)),
			StatusStyle(ansible.StatusChanged).Render(fmt.Sprintf("changed=%d", changed)),
			StatusStyle(ansible.StatusFailed).Render(fmt.Sprintf("failed=%d", failed)),
			StatusStyle(ansible.StatusUnreachable).Render(fmt.Sprintf("unreachable=%d", unreachable)),
		))
	}
	line := " " + strings.Join(parts, "   ")
	if v.err != nil && v.info.ExitCode < 0 && !errors.Is(v.err, context.Canceled) {
		line += "\n " + styleError.Render(firstLine(v.err.Error()))
	}
	return lipgloss.NewStyle().MaxWidth(v.width).Render(line)
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}

func (v *RunView) Title() string { return "run " + v.info.Label() }

func (v *RunView) KeyMap() []key.Binding {
	keys := []key.Binding{keyRunResult, keyRunOutput}
	if v.info.State.Active() {
		keys = append(keys, keyRunCancel)
	} else {
		keys = append(keys, keyRerun, keyRerunFail)
	}
	return append(keys, v.table.KeyMap()...)
}

// openResult drills down into the selected cell: the result of a task on a
// host, or of a task on every host when the task column is selected.
func (v *RunView) openResult() tea.Cmd {
	row, ok := v.table.Selected()
	if !ok {
		return nil
	}
	uuid, isTask := strings.CutPrefix(row.ID, "task:")
	if !isTask {
		return nil
	}
	task := strings.TrimSpace(ansi.Strip(v.labels[row.ID]))

	col := v.table.SelectedCol()
	if col == 0 {
		var b strings.Builder
		for _, host := range v.hosts {
			c := v.cells[uuid][host]
			if c == nil || c.last.Type == "" {
				continue
			}
			b.WriteString(Section("── "+host+" ") + renderCell(c) + "\n")
			b.WriteString(ColorJSON(c.last.Result) + "\n\n")
		}
		if b.Len() == 0 {
			return Info("no result yet for this task")
		}
		return Push(NewTextView(v.parent, task, StaticText(b.String())))
	}

	host := v.hosts[col-1]
	c := v.cells[uuid][host]
	switch {
	case c == nil:
		return Info(fmt.Sprintf("%s did not run on %s", task, host))
	case c.last.Type == "":
		return Info(fmt.Sprintf("%s is still running on %s", task, host))
	}
	return Push(NewTextView(v.parent, task+"@"+host, StaticText(describeResult(c))))
}

// describeResult renders a task result: a short header, then the module's
// result as highlighted JSON.
func describeResult(c *cell) string {
	e := c.last
	var b strings.Builder
	field := func(label, value string) {
		fmt.Fprintf(&b, "%s %s\n", Muted(fmt.Sprintf("%-8s", label)), value)
	}
	field("Task", e.Task+Muted("  ("+e.TaskAction+")"))
	field("Play", e.Play)
	field("Host", e.Host)
	field("Status", renderCell(c))
	field("Time", e.TS.Local().Format("15:04:05.000"))
	b.WriteString("\n" + Section("Result") + "\n")
	b.WriteString(ColorJSON(e.Result))
	return b.String()
}

func (v *RunView) SetSize(width, height int) {
	v.width = width
	v.table.SetSize(width, max(height-2, 0)) // summary may take two lines
}

func (v *RunView) SetFilter(f string) { v.table.SetFilter(f) }
func (v *RunView) Filter() string     { return v.table.Filter() }

// outputBuffer keeps the last max bytes of a run's raw output. It is written
// by the runner's goroutines and read by the UI.
type outputBuffer struct {
	mu  sync.Mutex
	max int
	buf []byte
}

func (b *outputBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if over := len(b.buf) - b.max; over > 0 {
		b.buf = append(b.buf[:0], b.buf[over:]...)
	}
	return len(p), nil
}

func (b *outputBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}
