# Factory backlog

`/factory next` runs the first unchecked item and ticks it with its job id.
One line per feature; the spec-writer turns it into a spec.

- [x] `:runs` refreshes on its own: a run started or finished elsewhere shows up without leaving the view.  → 001-runs-auto-refresh
- [ ] `:modules` view from `ansible-doc -l -j`, with `Enter` opening the module's documentation from `ansible-doc -j <module>`.
- [x] `:roles` view listing the project's roles, with `e` opening the role's `tasks/main.yml` in `$EDITOR`. → 002-roles-view
- [ ] `e` on a playbook opens it in `$EDITOR` (suspend and resume the terminal).
- [ ] Facts in a host's describe view: from the jsonfile fact cache when configured, otherwise `ansible <host> -m setup` on demand.
- [ ] Ad-hoc commands: `a` on a host or group asks for a module and arguments and streams the result like a run.
