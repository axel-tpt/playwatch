# `:runs` refreshes on its own

## Goal
While `:runs` is open, the list reloads itself every couple of seconds: a run
started or finished in another playwatch (or by `playwatch run` in another
terminal), and the state/duration/recap of a run of this session, show up
without pressing `ctrl+r` or leaving the view. This closes the "`:runs` ne se
rafraîchit pas seul" open point of CLAUDE.md.

## Behaviour
- `:runs` reloads every 2 s. Cursor (same row ID), filter `/` and scroll are
  kept across reloads (`Table.SetRows` already keeps the cursor by ID).
- `ctrl+r` still reloads immediately; it does not start a second refresh cycle.
- No reload is started while a previous load is still in flight (the cycle
  keeps going, the tick is just skipped).
- The cycle stops when the view leaves the stack (`:hosts`, `Esc` from a
  pushed list, quit / app context cancelled). While a view is pushed on top of
  `:runs` (e.g. `Enter` on a run), reloads may continue in the background; when
  the user comes back with `Esc` the list is already up to date.
- A failing auto reload behaves like a failing manual one: error in the status
  bar, previous rows kept, cycle continues.
- Other resources (`:hosts`, `:groups`, `:playbooks`) are unchanged: no auto
  refresh.
- `?` help: unchanged keys (`ctrl+r refresh` is still listed).

## Design
- `internal/ui/resourceview.go`: add an optional field to `ui.Resource`,
  `RefreshEvery time.Duration` (0 = never). `ResourceView.Init` returns the
  load plus, when `RefreshEvery > 0`, a `tea.Tick` producing an unexported
  `refreshTickMsg{view *ResourceView}` that implements `Targeted` (so the root
  model drops it once the view is gone and the cycle ends by itself, no change
  to `app.go`). On that message: if `v.ctx` is done, stop; otherwise start
  `v.load()` unless `v.loading`, and always re-arm the tick (batch). Only the
  tick handler re-arms; `ctrl+r` and `ui.Refresh()` only call `load()`.
- `internal/resource/runs.go`: set `RefreshEvery` to a named constant
  (`runsRefreshInterval = 2 * time.Second`). `Load` is unchanged: it already
  reads the session `RunList` and `store.List` off the event loop.
- No change to `internal/ansible` or `internal/store` (no new Ansible command;
  data comes from the run store and the in-memory `RunList`). No new
  dependency, no user config option (that is a separate step 6 item).
- Docs: `docs/history.md` and the `:runs` row of `docs/usage.md` mention the
  automatic refresh; remove the `:runs` item from "Points ouverts" in
  CLAUDE.md.

## Acceptance criteria
- [ ] AC1: a `ResourceView` with `RefreshEvery > 0` reloads on a
  `refreshTickMsg` and re-arms the tick; new rows returned by `Load` appear
  (verify: `go test ./internal/ui -run TestResourceViewAutoRefresh`)
- [ ] AC2: a tick received while a load is in flight does not call `Load` a
  second time but still re-arms (verify: `go test ./internal/ui -run TestResourceViewAutoRefreshSkipsWhileLoading`)
- [ ] AC3: with `RefreshEvery == 0`, `Init` schedules no tick and a stray tick
  does not reload (verify: `go test ./internal/ui -run TestResourceViewNoAutoRefresh`)
- [ ] AC4: cursor and filter survive an auto reload that adds a row above the
  selected one (verify: `go test ./internal/ui -run TestResourceViewAutoRefreshKeepsCursorAndFilter`)
- [ ] AC5: App-level: after `:runs`-like view is replaced via `:<other>`, its
  tick is dropped and `Load` is no longer called; after quit/cancelled ctx the
  tick does not re-arm (verify: `go test ./internal/ui -run TestAppStopsAutoRefresh`)
- [ ] AC6: the `runs` resource has `RefreshEvery > 0`, and a run created in the
  store after a first `Load` is returned by the next `Load` (verify:
  `go test ./internal/resource -run TestRunsAutoRefresh`, temp store dir)
- [ ] AC7: Tests drive ticks by sending the message directly (no real 2 s
  sleep); `go test -race ./...`, `go vet ./...` and `golangci-lint run` pass.
- [ ] AC8: `docs/history.md` and `docs/usage.md` describe the auto refresh;
  CLAUDE.md "Points ouverts" no longer lists `:runs` (verify: `grep -n refresh docs/history.md docs/usage.md`)

No integration test: no Ansible command is added.

## Out of scope
- Watching the store directory (fsnotify or similar): polling is enough.
- Configurable interval, a refresh toggle key, or auto refresh on other views.
- Any change to the root model (`app.go`), the store format or the event contract.
- Pushing live updates from `RunView` into the list (polling covers it).
