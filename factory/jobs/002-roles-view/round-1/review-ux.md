VERDICT: PASS

1. [nit] internal/resource/roles.go:60 The `e` help label "edit tasks" is fine. A later `e` on playbooks should reuse the same wording for consistency.
2. [nit] internal/resource/roles.go:38 If the Enter view of a role without tasks shows an error, the status bar already carries "role X has no tasks/main.yml". No dead end.

Checked: the `:roles` aliases, columns, Enter and `e` match the spec. `e` does not clash with the existing keys. Errors (unset `$EDITOR`, missing tasks file, non-zero editor exit) go to the status bar. The list refreshes after the editor exits. `docs/usage.md` and README are updated. The internal tests pass.
