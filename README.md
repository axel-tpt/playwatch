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

## Install

On Debian / Ubuntu:

```bash
wget https://github.com/axel-tpt/playwatch/releases/latest/download/playwatch_linux_amd64.deb && sudo apt install ./playwatch_linux_amd64.deb && rm playwatch_linux_amd64.deb
```

Other platforms and building from source: see [Installation](docs/installation.md). Ansible (`ansible-core`) must be in your `$PATH`.

## Quick start

Run playwatch from your Ansible project directory (playbooks in `./playbook`, or set another directory with `-p`):

```bash
cd my-ansible-project
playwatch -i inventory.ini
```

Switch views with `:hosts`, `:groups`, `:playbooks`, `:roles` and `:runs`, filter with `/`, press `r` on a playbook to run it, and `?` for help.

## Documentation

- [Installation](docs/installation.md)
- [Usage](docs/usage.md): options, views, keys, sample project
- [Running a playbook](docs/running-playbooks.md): run form, live run view, Vault
- [Command line](docs/cli.md): non-interactive subcommands
- [Run history](docs/history.md)
- [Contributing](CONTRIBUTING.md): build, tests and checks

## License

Copyright 2026 Axel TREPOUT

playwatch is licensed under the [Apache License 2.0](LICENSE).
