ESCALATE
Code, tests and checks are clean and all four reviews pass, but AC8 is only partly met: the CLAUDE.md "Points ouverts" line can't be removed on the branch (CLAUDE.md is gitignored), so a human has to edit it.

## Summary
Adds an optional `RefreshEvery time.Duration` to `ui.Resource`. `ResourceView` uses it to schedule a Targeted `refreshTickMsg` that reloads the rows, skips the reload while a load is in flight, and stops when the view leaves the stack or the context is cancelled.
`:runs` sets it to 2 s (`runsRefreshInterval`). Cursor and filter are kept across reloads, `ctrl+r` still works and does not start a second cycle, and other resources are unchanged.
New tests cover AC1 to AC6 (`internal/ui/ui_test.go`, `internal/resource/runs_test.go`), and `docs/history.md` and `docs/usage.md` now describe the refresh. Single commit 7a14c8e, no change to `app.go`, `internal/ansible`, `internal/store`, the event contract, dependencies or CI.

## Checks
- build ✅ (`go build ./...`)
- vet ✅ (`go vet ./...`)
- test -race ✅ (all packages pass)
- lint ✅ (`golangci-lint run`, 0 issues)
- merge-tree ✅ (`git merge-tree --write-tree main factory/001-runs-auto-refresh` gives a clean tree, no conflict)
- Round 1 reviews: security PASS, ux PASS, ui PASS, code PASS (nits only), no `rework-human.md`

## For the human (ESCALATE only)
The only open point is AC8. The spec asks to remove "`:runs` ne se rafraîchit pas seul" from "Points ouverts" in CLAUDE.md. CLAUDE.md is in `.gitignore` and does not exist in the worktree, so the builder could not change it, and agents should not edit the project instruction file on their own. Nothing else needs a decision: the code is in scope and every other criterion is met.

Ways to settle it:
- Accept the branch as is, then remove that item from "Points ouverts" in `/home/axelt/playwatch/CLAUDE.md` yourself: `/factory approve 001-runs-auto-refresh`
- Or, if CLAUDE.md should not change (or should change differently), say so: `/factory rework 001-runs-auto-refresh <note>`

Optional nit from ui-review, not blocking: on an empty history the placeholder switches between "nothing to show" and "loading…" on each 2 s reload. A follow-up could show "loading…" only before the first load.
