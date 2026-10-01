package ui

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/axel-tpt/playwatch/internal/ansible"
	"github.com/axel-tpt/playwatch/internal/store"
)

// send feeds msg to the app and runs the resulting commands synchronously,
// feeding their messages back. Commands that do not return quickly (timers,
// cursor blink) are dropped.
func send(t *testing.T, a *App, msg tea.Msg) {
	t.Helper()
	queue := []tea.Msg{msg}
	for steps := 0; len(queue) > 0; steps++ {
		if steps > 100 {
			t.Fatal("too many messages")
		}
		m := queue[0]
		queue = queue[1:]
		if batch, ok := m.(tea.BatchMsg); ok {
			for _, c := range batch {
				queue = append(queue, run(c)...)
			}
			continue
		}
		_, cmd := a.Update(m)
		queue = append(queue, run(cmd)...)
	}
}

func run(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	select {
	case m := <-ch:
		if m == nil {
			return nil
		}
		return []tea.Msg{m}
	case <-time.After(100 * time.Millisecond):
		return nil
	}
}

func press(t *testing.T, a *App, keys ...string) {
	t.Helper()
	for _, k := range keys {
		var msg tea.KeyPressMsg
		switch k {
		case "enter":
			msg = tea.KeyPressMsg{Code: tea.KeyEnter}
		case "esc":
			msg = tea.KeyPressMsg{Code: tea.KeyEscape}
		case "ctrl+c":
			msg = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
		case "down":
			msg = tea.KeyPressMsg{Code: tea.KeyDown}
		case "tab":
			msg = tea.KeyPressMsg{Code: tea.KeyTab}
		case " ":
			msg = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
		default:
			r := []rune(k)[0]
			msg = tea.KeyPressMsg{Code: r, Text: k}
		}
		send(t, a, msg)
	}
}

func typeText(t *testing.T, a *App, s string) {
	t.Helper()
	for _, r := range s {
		press(t, a, string(r))
	}
}

func staticResource(name string, ids ...string) Resource {
	return Resource{
		Name:    name,
		Columns: []Column{{Title: "name"}},
		Load: func(context.Context) ([]Row, error) {
			rows := make([]Row, len(ids))
			for i, id := range ids {
				rows[i] = Row{ID: id, Cells: []string{id}}
			}
			return rows, nil
		},
	}
}

func newTestApp(t *testing.T, resources ...Resource) *App {
	t.Helper()
	a := NewApp(context.Background(), Config{Resources: resources, Start: resources[0].Name, Runs: NewRunList()})
	send(t, a, tea.WindowSizeMsg{Width: 80, Height: 20})
	send(t, a, a.Init()())
	return a
}

func screen(a *App) string { return ansi.Strip(a.render()) }

func TestAppCommandSwitchesResource(t *testing.T) {
	a := newTestApp(t, staticResource("hosts", "web1", "web2"), staticResource("groups", "web", "db"))
	if got := a.top().Title(); got != "hosts(2)" {
		t.Fatalf("start title = %q, want hosts(2)", got)
	}

	press(t, a, ":")
	typeText(t, a, "groups")
	press(t, a, "enter")
	if got := a.top().Title(); got != "groups(2)" || len(a.stack) != 1 {
		t.Errorf("after :groups, stack = %d views, top %q", len(a.stack), got)
	}

	press(t, a, ":")
	typeText(t, a, "nope")
	press(t, a, "enter")
	if !a.statusErr || !strings.Contains(a.status, "unknown command") {
		t.Errorf("status = %q (err %v), want unknown command error", a.status, a.statusErr)
	}
}

func TestAppFilter(t *testing.T) {
	a := newTestApp(t, staticResource("hosts", "web1", "web2", "db1"))
	press(t, a, "/")
	typeText(t, a, "web")
	if got := a.top().Title(); got != "hosts(2)" {
		t.Errorf("live filter: title = %q, want hosts(2)", got)
	}
	press(t, a, "enter")
	if !strings.Contains(screen(a), "</web>") {
		t.Errorf("filter not shown in frame title:\n%s", screen(a))
	}
	press(t, a, "esc")
	if got := a.top().Title(); got != "hosts(3)" {
		t.Errorf("esc should clear the filter: title = %q", got)
	}
}

func TestAppDrillDownAndBack(t *testing.T) {
	res := staticResource("groups", "web")
	res.Enter = func(row Row) tea.Cmd {
		return Push(NewTextView(context.Background(), row.ID, StaticText("details of "+row.ID)))
	}
	a := newTestApp(t, res)
	press(t, a, "enter")
	if len(a.stack) != 2 || !strings.Contains(screen(a), "details of web") {
		t.Fatalf("enter should push the details view:\n%s", screen(a))
	}
	if !strings.Contains(screen(a), "groups(1) › web") {
		t.Errorf("breadcrumbs missing:\n%s", screen(a))
	}
	press(t, a, "esc")
	if len(a.stack) != 1 {
		t.Errorf("esc should pop: stack = %d", len(a.stack))
	}
	press(t, a, "esc")
	if len(a.stack) != 1 {
		t.Errorf("esc must not pop the last view")
	}
}

func TestAppLoadError(t *testing.T) {
	res := staticResource("hosts")
	res.Load = func(context.Context) ([]Row, error) { return nil, errors.New("[ERROR]: boom") }
	a := newTestApp(t, res)
	if !a.statusErr || a.status != "[ERROR]: boom" {
		t.Errorf("status = %q, want Ansible's error verbatim", a.status)
	}
}

func TestAppDropsMessagesForGoneViews(t *testing.T) {
	a := newTestApp(t, staticResource("hosts", "a"))
	gone := NewResourceView(context.Background(), staticResource("other", "x"), "")
	send(t, a, rowsLoadedMsg{view: gone, rows: []Row{{ID: "x"}}})
	if gone.table.Len() != 0 {
		t.Error("a view outside the stack received a message")
	}
}

func TestTableKeepsCursorOnRefresh(t *testing.T) {
	tb := NewTable(Column{Title: "name"})
	tb.SetSize(40, 10)
	tb.SetRows([]Row{{ID: "a", Cells: []string{"a"}}, {ID: "b", Cells: []string{"b"}}, {ID: "c", Cells: []string{"c"}}})
	tb.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	tb.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	tb.SetRows([]Row{{ID: "z", Cells: []string{"z"}}, {ID: "c", Cells: []string{"c"}}, {ID: "a", Cells: []string{"a"}}})
	if row, _ := tb.Selected(); row.ID != "c" {
		t.Errorf("selected = %q, want c", row.ID)
	}
	tb.SetFilter("zz")
	if _, ok := tb.Selected(); ok || tb.Len() != 0 {
		t.Error("filter matching nothing should leave no selection")
	}
}

func TestTableFitsWidth(t *testing.T) {
	tb := NewTable(Column{Title: "name"}, Column{Title: "desc"})
	tb.SetSize(30, 5)
	tb.SetRows([]Row{{ID: "a", Cells: []string{"a", strings.Repeat("x", 100)}}})
	for _, line := range strings.Split(tb.View(), "\n") {
		if w := ansi.StringWidth(line); w != 30 {
			t.Errorf("line width = %d, want 30: %q", w, line)
		}
	}
}

func TestColorJSON(t *testing.T) {
	in := `{"a":[1,-2.5e3,true,null,false],"b\"c":"x:y","n":12345678901234567890}`
	got := ansi.Strip(ColorJSON([]byte(in)))
	want := "{\n  \"a\": [\n    1,\n    -2.5e3,\n    true,\n    null,\n    false\n  ],\n  \"b\\\"c\": \"x:y\",\n  \"n\": 12345678901234567890\n}"
	if got != want {
		t.Errorf("ColorJSON stripped =\n%s\nwant\n%s", got, want)
	}
}

func TestRunViewMatrix(t *testing.T) {
	v := replayedRun(t)

	cellOf := func(task, host string) string {
		col := -1
		for i, h := range v.hosts {
			if h == host {
				col = i + 1
			}
		}
		for _, row := range v.table.Rows() {
			// A task name can appear in several plays: take the row
			// where the host has a status.
			if strings.TrimSpace(ansi.Strip(row.Cells[0])) == task && col > 0 && col < len(row.Cells) && row.Cells[col] != "" {
				return ansi.Strip(row.Cells[col])
			}
		}
		return "<missing>"
	}
	tests := []struct{ task, host, want string }{
		{"Pretend to reconfigure", "web1", "changed"},
		{"Loop over packages", "web2", "ok ×2"},
		{"Only on web1", "web2", "skipped"},
		{"Tolerated failure", "web1", "ignored"},
		{"Fail on web2", "web2", "failed"},
		{"handler: Pretend to reload", "web1", "ok"},
		{"Show role", "db1", "ok"},
	}
	for _, tt := range tests {
		if got := cellOf(tt.task, tt.host); got != tt.want {
			t.Errorf("cell(%s, %s) = %q, want %q", tt.task, tt.host, got, tt.want)
		}
	}
	if got := len(v.hosts); got != 3 {
		t.Errorf("hosts = %v, want 3 columns", v.hosts)
	}
	if v.info.Stats["web2"].Failures != 1 {
		t.Error("stats not recorded")
	}
	// Two plays, both rendered as separator rows.
	plays := 0
	for _, row := range v.table.Rows() {
		if strings.HasPrefix(row.ID, "play:") {
			plays++
		}
	}
	if plays != 2 {
		t.Errorf("got %d play rows, want 2", plays)
	}
}

func TestQuitAsksConfirmationWhileRunning(t *testing.T) {
	a := newTestApp(t, staticResource("hosts", "a"))
	v := NewRunView(context.Background(), RunEnv{Client: &ansible.Client{}, Runs: a.cfg.Runs}, "slow.yml", ansible.RunOptions{}, nil)
	v.info.State = RunRunning
	v.publish()

	_, cmd := a.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	msgs := run(cmd)
	if len(msgs) != 1 {
		t.Fatalf("ctrl+c returned %v, want a confirmation", msgs)
	}
	if _, ok := msgs[0].(confirmMsg); !ok {
		t.Fatalf("ctrl+c returned %T, want confirmMsg", msgs[0])
	}
	a.Update(msgs[0])
	_, cmd = a.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if cmd != nil || a.confirm != nil {
		t.Error("answering n should cancel quitting")
	}
}

func TestRunViewMarksRunningCellsAborted(t *testing.T) {
	v := NewRunView(context.Background(), testEnv(), "slow.yml", ansible.RunOptions{}, nil)
	v.SetSize(80, 10)
	v.apply(ansible.Event{Type: ansible.EventTaskStart, Task: "Wait", TaskUUID: "t1"})
	v.apply(ansible.Event{Type: ansible.EventRunnerStart, Task: "Wait", TaskUUID: "t1", Host: "web1"})
	v.info.State = RunRunning
	if _, handled := v.Interrupt(); !handled {
		t.Fatal("ctrl+c not handled by a running run")
	}
	v.finish(&runEnd{state: finalState(v.ctx.Err(), context.Canceled), exitCode: 99})
	if got := ansi.Strip(v.table.Rows()[0].Cells[1]); got != "aborted" {
		t.Errorf("cell = %q, want aborted", got)
	}
	if v.info.State != RunInterrupted {
		t.Errorf("state = %s, want interrupted", v.info.State)
	}
}

// replayedRun returns a run view fed with the events of a real site.yml run.
func replayedRun(t *testing.T) *RunView {
	t.Helper()
	data, err := os.ReadFile("../ansible/testdata/events_site.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	v := NewRunView(context.Background(), testEnv(), "site.yml", ansible.RunOptions{}, nil)
	v.SetSize(120, 30)
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		e, err := ansible.ParseEvent(sc.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		v.apply(e)
	}
	return v
}

// selectCell moves the run view's cursor to the given task (in its first
// play where it appears with a status for host) and host ("" = task column).
func selectCell(t *testing.T, v *RunView, task, host string) {
	t.Helper()
	col := 0
	if host != "" {
		col = slices.Index(v.hosts, host) + 1
	}
	for i, idx := range v.table.visible {
		row := v.table.rows[idx]
		if strings.TrimSpace(ansi.Strip(row.Cells[0])) != task {
			continue
		}
		if col > 0 && row.Cells[col] == "" {
			continue
		}
		v.table.cursor = i
		v.table.col = col
		return
	}
	t.Fatalf("no row for task %q", task)
}

// openedText runs the command returned by a key press and returns the text
// of the pushed view, or the status message.
func openedText(t *testing.T, v *RunView) string {
	t.Helper()
	cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		return ""
	}
	switch msg := cmd().(type) {
	case pushMsg:
		tv, ok := msg.view.(*TextView)
		if !ok {
			t.Fatalf("pushed %T, want *TextView", msg.view)
		}
		content, err := tv.load(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return tv.Title() + "\n" + ansi.Strip(content)
	case statusMsg:
		return "status: " + msg.text
	default:
		t.Fatalf("unexpected message %T", msg)
	}
	return ""
}

func TestRunViewCellResult(t *testing.T) {
	v := replayedRun(t)

	selectCell(t, v, "Fail on web2", "web2")
	got := openedText(t, v)
	for _, want := range []string{"Fail on web2@web2", "ansible.builtin.fail", "Status   failed", `"msg": "web2 is broken"`} {
		if !strings.Contains(got, want) {
			t.Errorf("result view lacks %q:\n%s", want, got)
		}
	}

	selectCell(t, v, "Secret task", "web1")
	if got := openedText(t, v); !strings.Contains(got, `"censored"`) || strings.Contains(got, "hunter2") {
		t.Errorf("no_log result not censored:\n%s", got)
	}

	selectCell(t, v, "Loop over packages", "web1")
	if got := openedText(t, v); !strings.Contains(got, `"item": "certbot"`) {
		t.Errorf("loop result lacks the items:\n%s", got)
	}
}

func TestRunViewTaskColumnShowsAllHosts(t *testing.T) {
	v := replayedRun(t)
	selectCell(t, v, "Fail on web2", "")
	got := openedText(t, v)
	if !strings.Contains(got, "── web1 skipped") || !strings.Contains(got, "── web2 failed") {
		t.Errorf("task view should list every host:\n%s", got)
	}
}

func TestRunViewEmptyCell(t *testing.T) {
	v := replayedRun(t)
	// "Only on web1" never ran on db1 (other play).
	selectCell(t, v, "Only on web1", "")
	v.table.col = slices.Index(v.hosts, "db1") + 1
	if got := openedText(t, v); !strings.HasPrefix(got, "status: ") {
		t.Errorf("empty cell should only show a status message, got %q", got)
	}
}

func TestTableCellCursor(t *testing.T) {
	tb := NewTable(Column{Title: "a"}, Column{Title: "b"}, Column{Title: "c"})
	tb.SetSize(40, 5)
	tb.SetRows([]Row{{ID: "1", Cells: []string{"x", "y", "z"}}})
	tb.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if tb.SelectedCol() != 0 {
		t.Error("right must not move without cell mode")
	}
	tb.EnableCellCursor()
	for range 5 {
		tb.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	}
	if tb.SelectedCol() != 2 {
		t.Errorf("col = %d, want clamped to 2", tb.SelectedCol())
	}
	tb.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	if tb.SelectedCol() != 1 {
		t.Errorf("col = %d after h, want 1", tb.SelectedCol())
	}
	tb.SetColumns(Column{Title: "a"})
	if tb.SelectedCol() != 0 {
		t.Errorf("col = %d after removing columns, want 0", tb.SelectedCol())
	}
}

func testEnv() RunEnv {
	return RunEnv{Client: &ansible.Client{}, Runs: NewRunList()}
}

// fakePlaybook installs a fake ansible-playbook replaying the site.yml events
// on fd 3 and exiting with code 2, like the real run.
func fakePlaybook(t *testing.T) {
	t.Helper()
	fixture, err := filepath.Abs("../ansible/testdata/events_site.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	script := "#!/bin/sh\necho 'PLAY [Web tier]'\ncat '" + fixture + "' >&3\nexit 2\n"
	if err := os.WriteFile(filepath.Join(dir, "ansible-playbook"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":/bin:/usr/bin")
}

func storeEnv(t *testing.T) RunEnv {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return RunEnv{Client: ansible.NewClient("inventory.ini"), Runs: NewRunList(), Store: st, Project: "/project"}
}

// runToEnd drives a live run view until the run is over.
func runToEnd(t *testing.T, v *RunView) {
	t.Helper()
	v.Update(v.start()())
	deadline := time.After(10 * time.Second)
	for v.info.State.Active() {
		select {
		case <-deadline:
			t.Fatal("run did not end")
		default:
		}
		v.Update(v.wait()())
	}
}

func TestRunIsRecordedAndReplayed(t *testing.T) {
	fakePlaybook(t)
	env := storeEnv(t)

	live := NewRunView(context.Background(), env, "site.yml", ansible.RunOptions{Check: true}, nil)
	live.SetSize(120, 30)
	live.info.Started = time.Now()
	runToEnd(t, live)
	<-live.done

	if live.info.State != RunFailed || live.info.ExitCode != 2 || live.info.StoreID == "" {
		t.Fatalf("live run = %+v", live.info)
	}
	meta, err := env.Store.Meta(live.info.StoreID)
	if err != nil {
		t.Fatal(err)
	}
	if meta.State != store.StateFailed || meta.ExitCode != 2 || meta.Stats["web2"].Failures != 1 ||
		meta.Project != "/project" || !meta.RunOptions().Check || !slices.Equal(meta.Args, []string{"--check"}) ||
		!slices.Equal(meta.Inventories, []string{"inventory.ini"}) {
		t.Errorf("stored meta = %+v", meta)
	}
	if out, _ := env.Store.Output(meta.ID); !strings.Contains(out, "PLAY [Web tier]") {
		t.Errorf("stored output = %q", out)
	}

	// History merges the live session run with the store without duplicates.
	history, err := env.History()
	if err != nil || len(history) != 1 || history[0].View != live {
		t.Fatalf("History() = %+v, %v", history, err)
	}

	replay := NewReplayView(context.Background(), env, infoFromMeta(meta))
	replay.SetSize(120, 30)
	replay.Update(replay.Init()())
	if got := ansi.Strip(replay.table.View()); got != ansi.Strip(live.table.View()) {
		t.Errorf("replayed matrix differs from the live one:\n%s\n---\n%s", got, ansi.Strip(live.table.View()))
	}
	if _, handled := replay.Interrupt(); handled {
		t.Error("a replay must not handle ctrl+c")
	}
}

func TestRerunFailed(t *testing.T) {
	env := testEnv()
	info := RunInfo{
		Playbook:    "site.yml",
		Inventories: []string{"prod.ini"},
		Options:     ansible.RunOptions{Check: true, Extra: []string{"--limit", "web", "-vv"}},
		Stats:       map[string]ansible.HostStats{"web1": {OK: 3}, "web2": {Failures: 1}},
	}
	push, ok := Rerun(context.Background(), env, info, true)().(pushMsg)
	if !ok {
		t.Fatal("Rerun(onlyFailed) should open the run form")
	}
	form := push.view.(*RunForm)
	opts := form.Options()
	if opts.Limit != "web2" || !opts.Check || !slices.Equal(opts.Extra, []string{"-vv"}) {
		t.Errorf("rerun options = %+v, want limit web2, check, and the old --limit dropped", opts)
	}
	if !slices.Equal(form.env.Client.Inventories, []string{"prod.ini"}) {
		t.Errorf("rerun must reuse the run's inventories, got %v", form.env.Client.Inventories)
	}
	if !strings.Contains(form.Command(), "-i prod.ini --limit web2 --check -vv site.yml") {
		t.Errorf("command = %q", form.Command())
	}

	info.Stats = map[string]ansible.HostStats{"web1": {OK: 3}}
	if _, ok := Rerun(context.Background(), env, info, true)().(statusMsg); !ok {
		t.Error("rerun on failed hosts without failures should only show a message")
	}
}

// formApp returns an app showing a run form for site.yml.
func formApp(t *testing.T, opts ansible.RunOptions) (*App, *RunForm) {
	t.Helper()
	a := newTestApp(t, staticResource("playbooks", "site.yml"))
	env := RunEnv{Client: ansible.NewClient("inventory.ini"), Runs: a.cfg.Runs}
	form := NewRunForm(context.Background(), env, "site.yml", opts)
	send(t, a, pushMsg{form})
	send(t, a, formNamesMsg{form: form, names: []string{"db", "web", "db1", "web1", "web2"}})
	return a, form
}

func TestRunFormEditsOptions(t *testing.T) {
	a, form := formApp(t, ansible.RunOptions{Tags: "deploy"})

	typeText(t, a, "web1:x/?") // : / ? are typed, not global keys
	if a.mode != promptNone || len(a.stack) != 2 {
		t.Fatalf("form keys leaked to the app: mode %d, stack %d", a.mode, len(a.stack))
	}
	if got := form.Options().Limit; got != "web1:x/?" {
		t.Errorf("limit = %q", got)
	}

	press(t, a, "down", "down", "down")
	typeText(t, a, "version=2")
	press(t, a, "down")
	press(t, a, " ") // check mode
	press(t, a, "down", "down")
	typeText(t, a, "hunter2")

	opts := form.Options()
	if opts.Tags != "deploy" || !opts.Check || opts.Diff || !slices.Equal(opts.ExtraVars, []string{"version=2"}) {
		t.Errorf("options = %+v", opts)
	}
	cmd := form.Command()
	if strings.Contains(cmd, "hunter2") || !strings.Contains(cmd, "--vault-password-file <typed password>") {
		t.Errorf("command must not show the password: %q", cmd)
	}
	if !strings.Contains(cmd, "--limit 'web1:x/?'") {
		t.Errorf("command should quote the limit: %q", cmd)
	}
	if screen := screen(a); strings.Contains(screen, "hunter2") {
		t.Errorf("password shown on screen:\n%s", screen)
	}

	_, c := a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	replace, ok := c().(replaceMsg)
	if !ok {
		t.Fatal("enter should replace the form with the run")
	}
	run := replace.view.(*RunView)
	if string(run.vaultPassword) != "hunter2" || !run.info.Options.AskVaultPass || !run.info.Options.Check {
		t.Errorf("run = %+v, password %q", run.info.Options, run.vaultPassword)
	}
	if form.fields[fieldVaultPassword].input.Value() != "" {
		t.Error("the form must forget the password once submitted")
	}
}

func TestRunFormLimitCompletion(t *testing.T) {
	a, form := formApp(t, ansible.RunOptions{})
	typeText(t, a, "db1,!we")
	press(t, a, "tab")
	if got := form.Options().Limit; got != "db1,!web" {
		t.Errorf("after tab, limit = %q, want db1,!web (first match)", got)
	}
	if form.focus != fieldLimit {
		t.Error("tab with a completion must stay on the limit field")
	}
	press(t, a, "tab")
	if form.focus != fieldTags {
		t.Error("tab without anything to complete moves to the next field")
	}
}

func TestRunFormEscCancels(t *testing.T) {
	a, _ := formApp(t, ansible.RunOptions{})
	press(t, a, "esc")
	if len(a.stack) != 1 {
		t.Errorf("esc should close the form, stack = %d", len(a.stack))
	}
}
