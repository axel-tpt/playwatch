VERDICT: PASS

The change has almost no visual surface: no new style, table, title, or help entry. It only reloads the existing `:runs` table periodically. No hard-coded colours, no layout change, and the `?` help is unchanged (`keyRefresh` is still listed).

1. [nit] internal/ui/resourceview.go:121-126 (`View`): while the table is empty, each auto reload sets `v.loading` and the text flips between " nothing to show" and " loading…" every 2 s. On an empty history this flickers. Optional fix: show " loading…" only before the first load completes (for example a `loaded bool` set in `rowsLoadedMsg`). Not blocking, because the load is fast and the table with rows is unaffected.
2. [nit] internal/ui/resourceview.go:100-110: with a persistent Load error, `Error(msg.err)` is re-sent every 2 s, so the status-bar message is refreshed continuously. The spec accepts this ("behaves like a failing manual reload"), so no change is required.
