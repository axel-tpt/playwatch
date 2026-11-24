# Build: 002-roles-view

## Round 1
- Commits: see `git log factory/002-roles-view` (single feat commit)
- Acceptance criteria:
  - AC1 ✅ TestParseRolesPath (internal/ansible)
  - AC2 ✅ TestRoleSearchDirs
  - AC3 ✅ TestListRoles
  - AC4 ✅ TestRoleTasksFile
  - AC5 ✅ TestEditorCommand, TestEditorUnset
  - AC6 ✅ TestListPlaybooks unchanged and green after playbookDir extraction
  - AC7 ✅ TestIntegrationRolesPath, TestIntegrationRoles
  - AC8 ✅ vet, lint, test -race; `go list -deps ... | grep -c charm.land` = 0
  - AC9 ✅ grep shows `:roles` in docs/usage.md and README.md
- Checks: build ✅ vet ✅ test -race ✅ lint ✅ integration ✅
- Notes: config_dump.json fixture is real output with local paths anonymised.
  `internal/ansible/integration_test.go` has a pre-existing gofmt misalignment (untouched).
  Symlinked role directories are followed.
