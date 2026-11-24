# Build: 001-runs-auto-refresh

## Round 1
- Commits: see `git log` on factory/001-runs-auto-refresh (feat(ui): refresh :runs automatically every 2 s)
- Acceptance criteria:
  - AC1 ✅ TestResourceViewAutoRefresh
  - AC2 ✅ TestResourceViewAutoRefreshSkipsWhileLoading
  - AC3 ✅ TestResourceViewNoAutoRefresh
  - AC4 ✅ TestResourceViewAutoRefreshKeepsCursorAndFilter
  - AC5 ✅ TestAppStopsAutoRefresh
  - AC6 ✅ TestRunsAutoRefresh (internal/resource/runs_test.go)
  - AC7 ✅ ticks sent directly; go test -race -timeout 60s ./..., vet, lint pass
  - AC8 ⚠️ docs/history.md and docs/usage.md updated; CLAUDE.md "Points ouverts" NOT changed (see notes)
- Checks: build ✅ vet ✅ test -race ✅ lint ✅ integration n/a
- Notes: CLAUDE.md is not present in the worktree (untracked/ignored), and
  editing the main checkout is forbidden, so the "Points ouverts" item
  (`:runs` ne se rafraîchit pas seul) still has to be removed by the
  orchestrator/human. Tests never execute the tea.Tick cmd (it blocks for the
  interval); the tick-while-loading test only checks that no batch is returned.
  gofmt flags the pre-existing internal/ansible/integration_test.go; left untouched.
