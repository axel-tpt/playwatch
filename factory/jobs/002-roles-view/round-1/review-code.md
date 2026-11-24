VERDICT: PASS

Commands (in .worktrees/002-roles-view):
- ✅ go build ./...
- ✅ go vet ./...
- ✅ go test -race ./...
- ✅ golangci-lint run (0 issues)
- ✅ go test -tags integration ./...
- ✅ go list -deps ./internal/ansible ./internal/store | grep -c charm.land -> 0
- ✅ grep ':roles' docs/usage.md README.md (both match)

All AC1-AC9 are met and covered by tests. Architecture is respected: config.go has no UI import, roles live in their own file, app.go is untouched, and ansible-config JSON is used. Errors are wrapped with %w, and there is no panic. Single Conventional Commit.

Findings:
1. [nit] internal/resource/roles.go:70-76: roleSearchDirs uses filepath.Abs, which resolves against the process cwd, not the project dir. Relative roles_path entries and a relative env.Dir are only correct when the project dir is the process cwd. The spec says Ansible resolves relative paths itself, so this is acceptable. Consider joining relative entries to projectDir for robustness.
