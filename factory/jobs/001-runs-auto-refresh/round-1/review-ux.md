VERDICT: PASS

1. [nit] CLAUDE.md "Points ouverts" still lists `:runs` ne se rafraichit pas seul (AC8). The worktree has no CLAUDE.md, so the orchestrator must remove it at merge.
2. [nit] internal/ui/resourceview.go:97-104: a persistent load failure puts the same error in the status bar every 2 s. This matches the spec ("behaves like a manual reload, cycle continues"), so no change is needed.
3. [nit] internal/ui/resourceview.go:93-110: the "loading..." text only shows when the table is empty, so the 2 s reload does not flicker. Cursor and filter are kept through SetRows. Keys and `?` help are unchanged. `ctrl+r` only calls load(), so there is no second cycle.
4. Docs: docs/history.md and docs/usage.md describe the 2 s refresh.
