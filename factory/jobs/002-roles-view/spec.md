# `:roles` view

## Goal
`:roles` lists the roles Ansible would find for this project (the project's
`roles/` directories plus the configured `roles_path`), and `e` opens the
selected role's `tasks/main.yml` in `$EDITOR`, suspending the TUI and coming
back to it when the editor exits. It also adds the reusable `$EDITOR`
mechanism that later step 6 items (`e` on a playbook) will use.

## Behaviour
- `:roles` (aliases `role`, `ro`) is listed in `:` autocompletion and `?` help.
  Columns: `name`, `tasks` (`main.yml` / `main.yaml` / `-`), `path` (the
  directory holding the role; relative to the project when inside it, else
  absolute).
- Search directories, in Ansible's lookup order, duplicates removed (same
  absolute path): `<playbook dir>/roles` (the directory `:playbooks` lists,
  same `-p` / `./playbook` / root fallback), `<project>/roles`, then each entry
  of `DEFAULT_ROLES_PATH`. Missing directories are skipped silently.
- A role is any non-hidden subdirectory of a search directory. Nothing inside
  it is parsed. A role name present in several directories shows one row per
  directory. Rows are in search order, then by name.
- `Enter`: read-only `TextView` of the role's tasks file (title
  `<role>/tasks/main.yml`), `Esc` back.
- `e`: opens the tasks file in `$EDITOR` via Bubble Tea's exec (terminal
  suspended, then restored). `$EDITOR` is split on whitespace (`code -w` works)
  and run without a shell. After the editor exits the list is refreshed.
- Errors (status bar, view stays usable): `$EDITOR is not set`; `role <name>
  has no tasks/main.yml`; editor exits non-zero → `editor: <err>`;
  `ansible-config` failure → its stderr verbatim (as for other Ansible errors).
- `/` filter and `ctrl+r` work as on every resource list.

## Design
- `internal/ansible/config.go` (new): `(*Client).RolesPath(ctx) ([]string,
  error)` runs `ansible-config dump --format json` through `Client.output`
  (cwd = project, so its `ansible.cfg` is honoured; Ansible resolves relative
  paths itself), parsed by a pure `parseRolesPath([]byte)` reading the
  `DEFAULT_ROLES_PATH` entry (`[{name, origin, value}]`, `value` a list).
  Fixture: `internal/ansible/testdata/config_dump.json` (real output). No UI
  import.
- `internal/resource/playbooks.go`: extract the playbook-directory resolution
  of `listPlaybooks` into `playbookDir(projectDir, dir string) string`, reused
  by roles; `listPlaybooks` behaviour unchanged.
- `internal/resource/roles.go` (new): `Roles(env)` resource; pure helpers
  `roleSearchDirs(projectDir, pbDir string, rolesPath []string) []string`,
  `listRoles(projectDir string, dirs []string) ([]ui.Row, error)` (row ID =
  absolute role dir) and `roleTasksFile(roleDir string) (string, error)`
  (`tasks/main.yml`, then `tasks/main.yaml`). `Load` calls `RolesPath` then
  lists off the event loop. Register it in `All` after `Playbooks`.
- `internal/ui/editor.go` (new): `Edit(path string) tea.Cmd` using
  `tea.ExecProcess`; on exit returns an error status or `Refresh()`. Pure
  `editorCommand(editor, path string) (*exec.Cmd, error)` builds the command
  (error when `editor` is blank). `Edit` reads `$EDITOR` when called. No
  change to `app.go`.
- `testdata/project/roles/demo/tasks/main.yml`: one `debug` task (localhost
  safe, not used by any playbook).
- Docs: `docs/usage.md` (Views table row for `:roles`, note on `$EDITOR`),
  README line 49 view list.

## Acceptance criteria
- [ ] AC1: `parseRolesPath` returns the `DEFAULT_ROLES_PATH` list from the
  fixture and an error on invalid JSON or a missing entry (verify: `go test ./internal/ansible -run TestParseRolesPath`)
- [ ] AC2: `roleSearchDirs` orders playbook-dir `roles`, project `roles`, then
  roles_path, and drops duplicates (verify: `go test ./internal/resource -run TestRoleSearchDirs`)
- [ ] AC3: `listRoles` lists subdirectories only, skips hidden entries and
  missing dirs, fills `tasks` and `path` columns, keeps duplicate names from
  different dirs (verify: `go test ./internal/resource -run TestListRoles`, `t.TempDir`)
- [ ] AC4: `roleTasksFile` prefers `main.yml`, falls back to `main.yaml`, errors
  with the role name when neither exists (verify: `go test ./internal/resource -run TestRoleTasksFile`)
- [ ] AC5: `editorCommand("code -w", p)` yields args `[code -w p]`; blank
  editor returns an error; `Edit` with `$EDITOR` unset yields an error status
  message (verify: `go test ./internal/ui -run TestEditor`)
- [ ] AC6: `playbookDir` refactor keeps existing behaviour (verify: `go test ./internal/resource -run TestListPlaybooks`)
- [ ] AC7: integration: `RolesPath` on `testdata/project` returns a non-empty
  list, and the `roles` resource `Load` there returns a `demo` row with
  `tasks = main.yml` (verify: `go test -tags integration ./internal/ansible -run TestIntegrationRolesPath` and `go test -tags integration ./internal/resource -run TestIntegrationRoles`)
- [ ] AC8: `go vet ./... && golangci-lint run && go test -race ./...` pass;
  `internal/ansible` and `internal/store` import no `charm.land` package
  (verify: `go list -deps ./internal/ansible ./internal/store | grep -c charm.land` prints 0)
- [ ] AC9: `docs/usage.md` lists `:roles` with `Enter` and `e`, and mentions
  `$EDITOR`; README view list includes `:roles` (verify: `grep -n ':roles' docs/usage.md README.md`)

## Out of scope
- Roles from collections (`ansible-galaxy collection list`), `ansible-galaxy
  role list`, `ansible-doc -t role` argument specs.
- Parsing any role file (meta, defaults, dependencies) or `ansible.cfg`.
- Creating a missing `tasks/main.yml`; editing other role files.
- `e` on playbooks or other resources (separate backlog item; only provide
  `ui.Edit`). A `playwatch roles` CLI subcommand. Fallback to `$VISUAL`/`vi`.
- Merging `callback_plugins` via `ansible-config dump` (separate open point).
