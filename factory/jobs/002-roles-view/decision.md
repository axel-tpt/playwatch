APPROVE
All four round-1 reviews pass, AC1-AC9 are met, checks are green, the change stays within the spec, and it merges cleanly into main.

## Summary
Adds a `:roles` resource (aliases `role`, `ro`) listing role directories from `<playbook dir>/roles`, `<project>/roles` and `DEFAULT_ROLES_PATH` (read through `ansible-config dump --format json`), with `Enter` showing `tasks/main.yml` and `e` opening it in `$EDITOR`.
Adds a reusable `ui.Edit` (via `tea.ExecProcess`, `$EDITOR` split on whitespace, no shell) and moves playbook-directory resolution into `playbookDir` without changing behaviour.
Includes unit tests, integration tests, a real `config_dump.json` fixture with anonymised paths, a `testdata/project/roles/demo` role, and doc updates in `docs/usage.md` and README. There is one commit, `d716d35`.

## Checks
- build ✅ (`go build ./...`)
- vet ✅ (`go vet ./...`)
- test -race ✅ (`go test -race -count=1 ./...`). The integration tests `TestIntegrationRolesPath` and `TestIntegrationRoles` also pass.
- lint ✅ (`golangci-lint run`: 0 issues)
- merge-tree ✅ (`git merge-tree --write-tree main factory/002-roles-view`: clean, tree `c8094fc`)
- Architecture ✅ (`go list -deps ./internal/ansible ./internal/store | grep -c charm.land` = 0)
- No escalation trigger ✅ (no go.mod/go.sum, CI, release or packaging change; event contract, callback plugin, secrets handling and run store are untouched)

Non-blocking nits from the reviews could become follow-ups:
- `roleSearchDirs` resolves relative paths against the process cwd, not the project dir.
- `roleTasksFile` accepts non-regular files such as FIFOs.
- Future callers of `ui.Edit` must pass absolute paths so that a file name starting with `-` is never read as an editor flag.
