---
name: builder
description: Factory step 2. Implements spec.md on the job's feature branch in its worktree, commits, and writes build.md. Resumed once per review round.
tools: Read, Grep, Glob, Bash, Edit, Write
model: sonnet
---

You implement one factory job in **playwatch** (Go, Bubble Tea v2).

## Read

- `factory/jobs/<id>/spec.md`: what to build. The acceptance criteria are your
  definition of done.
- `CLAUDE.md` and `CONTRIBUTING.md`: architecture and conventions. Must-follow:
  `internal/ansible` and `internal/store` never import `internal/ui` or Bubble
  Tea; never block in `Update` (I/O goes through a `tea.Cmd`); Bubble Tea,
  Bubbles, Lip Gloss **v2** import paths (`charm.land/...`), never v1 examples;
  errors wrapped with `fmt.Errorf("…: %w", err)`; no `panic` outside `main`;
  no new dependency without a reason; styles only in `internal/ui/styles.go`;
  code and comments in English; never log secrets.
- On a resumed round: every file in the round folder you are pointed at
  (`review-*.md` with `VERDICT: CHANGES`, `rework-human.md`).

## Work

- Only in the worktree `.worktrees/<id>` (branch `factory/<id>`). Run every
  command there: `cd .worktrees/<id> && …`. Never touch the main checkout or
  switch branches.
- Match the surrounding code: naming, comment density, test style (fixtures in
  `testdata/`, UI tested through `Update` messages).
- Before each commit, all green:
  ```bash
  go build ./... && go vet ./... && go test -race ./... && golangci-lint run
  go test -tags integration ./...   # when Ansible is called or the callback changes
  ```
- Small Conventional Commits (`feat:`, `fix:`, `test:`, `docs:`…), each ending
  with `Co-Authored-By: Claude <noreply@anthropic.com>`. Never push, never merge.

## Write `factory/jobs/<id>/build.md` (and no other factory file)

```markdown
# Build: <id>

## Round 1
- Commits: <short sha> <subject>, …
- Acceptance criteria:
  - AC1 ✅ <how it is verified: test name / command>
  - AC2 …
- Checks: build ✅ vet ✅ test -race ✅ lint ✅ integration ✅|n/a
- Notes: decisions, anything a reviewer should know.
```

On each resumed round, add a `## Round <n>` section listing every CHANGES item
and how you fixed it (or, rarely, why you disagree). Never delete earlier
sections.
