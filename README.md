# playwatch

A terminal UI for Ansible, inspired by [k9s](https://k9scli.io/).

Browse your inventory, run playbooks and watch them live as a task × host matrix, drill into any result, and keep a history of every run, all from the keyboard.

```
┌─ playbooks(3) › run site.yml@22:45:29 ─────────────────────────────────────┐
│ playbook site.yml   state failed (rc=2)   time 700ms   ok=13 changed=2 …   │
│ TASK                          WEB1     WEB2     DB1                        │
│ ▸ Web tier                                                                 │
│   Show role                   ok       ok                                  │
│   Pretend to reconfigure      changed  changed                             │
│   Loop over packages          ok ×2    ok ×2                               │
│   Tolerated failure           ignored  ignored                             │
│   Fail on web2                skipped  failed                              │
│   handler: Pretend to reload  ok                                           │
│ ▸ Database tier                                                            │
│   Show role                                     ok                         │
└────────────────────────────────────────────────────────────────────────────┘
```

## Why playwatch

- **k9s-style navigation.** Each resource has its own view (hosts, groups, playbooks, runs). You switch with `:`, filter with `/`, open with `Enter` and go back with `Esc`.
- **Ansible is the source of truth.** playwatch never parses your inventory or config itself. Everything comes from Ansible's own JSON output (`ansible-inventory`, …), so dynamic inventories, plugins and variable precedence just work.
- **Structured run events.** A small callback plugin, embedded in the binary, streams one JSON event per task result. playwatch never scrapes `ansible-playbook`'s text output.
- **Run history.** Every run is saved locally. You can replay it, rerun it, or rerun it only on the hosts that failed.

## Requirements

- Linux or macOS
- Ansible (`ansible-core`) in your `$PATH`. playwatch needs no other Python dependency.
- Go 1.27 or later, to build from source

## Installation

From a clone of the repository:

```bash
go install ./cmd/playwatch
```

This installs the `playwatch` binary in `$(go env GOPATH)/bin` (usually `~/go/bin`), which must be in your `$PATH`.

Once the repository is published, you will also be able to run:

```bash
go install github.com/axel-tpt/playwatch/cmd/playwatch@latest
```

## Usage

Run playwatch from your Ansible project directory, where your playbooks are:

```bash
cd my-ansible-project
playwatch -i inventory.ini
```

`-i` can be repeated. Without it, playwatch uses your Ansible configuration (`ansible.cfg`, `ANSIBLE_INVENTORY`).

To try playwatch without a project of your own, use the sample project. All of its hosts are aliases of your own machine, and it only runs `debug` tasks:

```bash
cd testdata/project
playwatch -i inventory.ini
```

### Views

| Command | Shows | Main keys |
|---|---|---|
| `:hosts` | Inventory hosts | `Enter`/`d`: groups, variables and recent runs of the host |
| `:groups` | Inventory groups | `Enter`: hosts of the group, subgroups included |
| `:playbooks` | YAML files of the project | `Enter`: hosts and tasks the playbook targets · `r`: run |
| `:runs` | Run history of the project | `Enter`: open · `r`: rerun · `f`: rerun on failed hosts · `ctrl+d`: delete |

### Keys

| Key | Action |
|---|---|
| `:` | Switch to a view (`Tab` completes, `:q` quits) |
| `/` | Filter the current view |
| `Enter` / `Esc` | Open / go back (`Esc` clears the filter first) |
| `↑↓` `j k`, `g G` | Move, go to top/bottom |
| `ctrl+r` | Refresh |
| `?` | Help for the current view |
| `ctrl+c` | Interrupt the current run, or quit |

### Running a playbook

Press `r` on a playbook. A form opens, prefilled with the options of that playbook's last run:

- limit (`Tab` completes host and group names)
- tags and skip tags
- extra vars
- check mode and diff
- a vault password or a vault password file

The exact `ansible-playbook` command is shown as you type. Press `Enter` to run it.

The run view fills in live as Ansible works:

- `←→` and `↑↓` move between cells.
- `Enter` on a cell shows the full JSON result of that task on that host. On the task column, it shows the result for every host.
- `o` shows Ansible's raw output.
- `ctrl+c` interrupts the run cleanly.
- `Esc` leaves the run going in the background. Get back to it from `:runs`.

A vault password typed into the form is handed to Ansible through a pipe. It is never written to disk or saved in the history.

### Command line

The same features are available without the TUI:

```bash
playwatch -i inventory.ini hosts            # list hosts
playwatch -i inventory.ini groups           # list groups
playwatch -i inventory.ini host web1        # variables of a host, as JSON
playwatch -i inventory.ini run site.yml -l web -C -D -e "version=2"
playwatch runs                              # run history of the current project
```

`playwatch run` accepts the usual `ansible-playbook` options: `-l`, `-t`, `--skip-tags`, `-C`, `-D`, `-e`, `--vault-password-file` and `--vault-id`. It prints one line per task result (`--json` for raw events, `--raw` for Ansible's own output) and exits with Ansible's exit code.

### Run history

Runs are saved under `$XDG_STATE_HOME/playwatch/runs` (by default `~/.local/state/playwatch/runs`), readable only by you. Each run keeps its options, its events and Ansible's raw output. The 200 most recent runs are kept.

## Development

```bash
go build ./cmd/playwatch                    # build
go test ./...                         # unit tests
go test -tags integration ./...       # integration tests (need Ansible; localhost only)
go vet ./... && golangci-lint run     # checks
```

## Roadmap

- [x] Inventory: hosts, groups, host description
- [x] Live runs with a task × host matrix
- [x] Drill-down into task results
- [x] Run history, replay and reruns
- [x] Run options: limit, tags, check, diff, extra vars, Vault
- [ ] Extras: ad-hoc commands, `:modules` (ansible-doc), `:roles`, facts, `$EDITOR`, user configuration

## License

Copyright 2026 Axel TREPOUT

playwatch is licensed under the [Apache License 2.0](LICENSE).
