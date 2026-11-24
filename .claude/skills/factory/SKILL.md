---
name: factory
description: Software factory for playwatch. Runs a feature through spec → build → 4 parallel reviews → approval → merge, using the agents in .claude/agents and the state in factory/. Use for /factory <feature>, /factory next, /factory approve <job-id>, /factory rework <job-id> <note>.
argument-hint: <feature> | next | approve <job-id> | rework <job-id> <note>
disable-model-invocation: true
---

# /factory: you are the orchestrator

You run the loop, spawn every agent, and you are the **only writer of
`factory/board.json`**. You never write code, specs or reviews yourself: agents
do, each into its own file. Agents get their context from files, never from
this conversation, so every prompt you send is just paths.

```
feature → spec-writer → builder ⇄ 4 reviewers (security, ux, ui, code) → approver → merge | needs-human
```

## Names and paths

| Thing | Value |
|---|---|
| Job id | `NNN-slug`: next number after the highest id in the board, zero-padded to 3, slug = 2–4 kebab words from the feature |
| Job folder | `factory/jobs/<id>/` |
| Branch | `factory/<id>` |
| Worktree | `.worktrees/<id>/` (the main checkout always stays on `main`) |
| Builder name | `builder-<id>` |
| Reviewers | `security-reviewer`, `ux-reviewer`, `ui-reviewer`, `code-reviewer` → `round-<n>/review-{security,ux,ui,code}.md` |
| `MAX_RESUMES` | `2`: automatic builder resumes per entry into the review loop |

In agent prompts, write every path as an absolute path from the repo root
(`git rev-parse --show-toplevel`): agents' shells may change directory.

## Board

`factory/board.json` drives `factory/dashboard.html`. Read it, change one job,
write the whole file back. Create it as `{"jobs": []}` if missing. Update it at
**every** transition below, so the dashboard moves while you work.

```json
{ "jobs": [ {
  "id": "003-rate-limiting", "title": "Short feature title",
  "branch": "factory/003-rate-limiting",
  "stage": "review",
  "round": 2,
  "rounds": [
    { "security": "CHANGES", "ux": "PASS", "ui": "PASS", "code": "PASS" },
    { "security": "pending", "ux": "PASS", "ui": "pending", "code": "PASS" }
  ],
  "decision": null,
  "note": "round 2: waiting for security, ui",
  "updated": "2026-10-24T21:03:00+02:00"
} ] }
```

- `stage` is one of `spec`, `build`, `review`, `approve`, `needs-human`, `merged`.
- `rounds[n-1]` holds round n's verdicts: `pending`, `PASS` or `CHANGES`. A
  human rework adds `"human": "CHANGES"` to the latest round.
- `decision` is `null`, `APPROVE` or `ESCALATE`. `note` is one line saying what
  is happening now. `updated` comes from `date -Is`.

## Preflight (every command)

1. `git branch --show-current` must print `main` in the repo root. If not, stop
   and tell the user.
2. Make sure `factory/board.json` exists.

## `/factory <feature>`

1. **Create the job.** Pick the id, `mkdir -p factory/jobs/<id>`, write the
   feature text verbatim to `factory/jobs/<id>/feature.md` (with a `# <title>`
   line on top). Add the job to the board: `stage: spec`, `round: 0`,
   `rounds: []`.
2. **Spec.** Spawn `spec-writer` with: `Job <id>. Read factory/jobs/<id>/feature.md, write factory/jobs/<id>/spec.md.`
   When it returns, read the first line of `spec.md`. If it starts with
   `BLOCKED:`, set `stage: needs-human`, `note` = the reason, and stop.
3. **Branch.** `git worktree add .worktrees/<id> -b factory/<id> main`.
4. **Build.** Set `stage: build`. Spawn `builder` with name `builder-<id>` and:
   `Job <id>. Spec: factory/jobs/<id>/spec.md. Work and commit in .worktrees/<id> (branch factory/<id>). Write factory/jobs/<id>/build.md.`
   Then run **the review loop**.

### The review loop

`resumes = 0` on entry.

1. **Open round n** (`n = round + 1`): `mkdir -p factory/jobs/<id>/round-<n>`, set
   `round: n`, `stage: review`, append `{security, ux, ui, code: "pending"}` to `rounds`.
2. **Spawn the 4 reviewers in one message** (in parallel, in the background),
   each with:
   `Job <id>, round <n>. Spec: factory/jobs/<id>/spec.md. Build notes: factory/jobs/<id>/build.md. Code: branch factory/<id>, worktree .worktrees/<id>. Earlier rounds: factory/jobs/<id>/round-*/. Write factory/jobs/<id>/round-<n>/review-<kind>.md.`
3. **As each reviewer finishes**, read the first line of its file
   (`VERDICT: PASS` or `VERDICT: CHANGES`) and set that chip on the board right
   away. If an agent finishes without writing its file, spawn it once more; if
   the file is still missing, set `stage: needs-human` with a note and stop.
4. **WAIT FOR ALL.** The round is done only when all 4 files exist in
   `round-<n>/`. Never message the builder while a round is open.
5. **Any CHANGES and `resumes < MAX_RESUMES`:** `resumes += 1`, set
   `stage: build`, and resume the same builder:
   `SendMessage(to: "builder-<id>")`: `Round <n> reviews are in factory/jobs/<id>/round-<n>/. Fix every CHANGES item, commit, and add a "## Round <n+1>" section to build.md.`
   When it returns, go back to step 1.
6. **All PASS, or resumes exhausted:** go to **approval**.

If `builder-<id>` is not reachable (new session, see `ListAgents`), spawn a
fresh `builder` with the same name and the same message plus `Read build.md first: you are continuing this job.`

### Approval

Set `stage: approve`. Spawn `approver` with:
`Job <id>. Read everything in factory/jobs/<id>/ and the diff of factory/<id> against main. Write factory/jobs/<id>/decision.md.`
If resumes were exhausted with CHANGES still open, add: `Review rounds are exhausted with open CHANGES: you must ESCALATE.`

Read the first word of `decision.md` and set `decision`.

- **APPROVE** → **merge** (below).
- **ESCALATE** → `stage: needs-human`, `note` = the decision's one-line reason.
  Tell the user: `/factory approve <id>` or `/factory rework <id> <note>`.

### Merge

From the repo root, on `main`:

```bash
git merge --no-ff factory/<id> -m "Merge factory/<id>: <title>" -m "Co-Authored-By: Claude <noreply@anthropic.com>"
go build ./... && go test ./...
git worktree remove .worktrees/<id> && git branch -d factory/<id>
```

If the merge conflicts: `git merge --abort`, set `stage: needs-human`,
`note: merge conflict with main`, stop. If tests fail after the merge, keep the
merge, set `note: merged, tests failing on main` and tell the user. Otherwise
`stage: merged`. Never push.

## `/factory next`

Take the first `- [ ]` line in `factory/backlog.md`. Tick it right away
(`- [x] … → <id>`) so it is never picked twice, then run it as
`/factory <that line's text>`. If nothing is unchecked, say so.

## `/factory approve <job-id>`

Only for a job in `needs-human`. A human is overriding the approver: run
**merge** directly. Append `Approved by a human on <date>.` to `decision.md`
(create it with `APPROVE` on the first line if it does not exist) and set
`decision: APPROVE`.

## `/factory rework <job-id> <note>`

For any job not `merged`. Write the note to
`factory/jobs/<id>/round-<round>/rework-human.md` (first line
`VERDICT: CHANGES`, then the note verbatim), set `rounds[round-1].human = "CHANGES"`,
`stage: build`, `decision: null`. If the job has no worktree yet (blocked at
spec), recreate the job from step 2 instead, adding the note to `feature.md`.
Otherwise resume `builder-<id>` pointing it at that round folder, as in loop
step 5, then run **the review loop** again with a fresh `resumes = 0`.

## Finishing

End every command with one line per job touched: id, stage, and the next
command the user can run. The dashboard is at `factory/dashboard.html`
(`cd factory && python3 -m http.server 8000`).
