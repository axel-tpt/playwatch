VERDICT: PASS

Commands (in .worktrees/001-runs-auto-refresh):
- ✅ go build ./...
- ✅ go vet ./...
- ✅ go test -race -timeout 120s ./...
- ✅ golangci-lint run (0 issues)
- n/a integration tests (no Ansible command added)

Acceptance criteria: AC1-AC7 met and covered by the named tests (all pass). AC8: docs/history.md and docs/usage.md updated; the CLAUDE.md part is not doable on the branch (see 1).

Findings:
1. [nit] CLAUDE.md:135 still lists "`:runs` ne se rafraîchit pas seul" under "Points ouverts". CLAUDE.md is gitignored (.gitignore:6) and absent from the worktree, so the builder could not change it. The orchestrator must remove that item in the main checkout at merge time.
2. [nit] internal/ui/ui_test.go, TestResourceViewAutoRefreshSkipsWhileLoading: the 100 ms timeout branch can't tell a bare tick from a hang, so the test is weak. The `calls != 0` check still guards the main behaviour. No change required.
3. [nit] internal/ui/resourceview.go, `refreshTickMsg` handler: the `v.res.RefreshEvery <= 0` check is redundant with `tick()`. It is cheap and AC3 asks for the stray-tick guard, so keep it.

Architecture: no change to app.go, ansible or store; the tick is Targeted; the load runs in a tea.Cmd; the context is checked; ctrl+r doesn't re-arm. Commit message follows Conventional Commits.
