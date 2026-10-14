# Usage

Run playwatch from your Ansible project directory:

```bash
cd my-ansible-project
playwatch -i inventory.ini
```

## Options

| Option | Meaning |
|---|---|
| `-i`, `--inventory` | Inventory source, repeatable. Without it, playwatch uses your Ansible configuration (`ansible.cfg`, `ANSIBLE_INVENTORY`). |
| `-p`, `--playbook-dir` | Directory holding the playbooks, relative to the current directory or absolute. Defaults to `./playbook`, or the current directory if `./playbook` does not exist. |

## Sample project

To try playwatch without a project of your own, use the sample project. All of its hosts are aliases of your own machine, and it only runs `debug` tasks:

```bash
cd testdata/project
playwatch -i inventory.ini
```

## Views

| Command | Shows | Main keys |
|---|---|---|
| `:hosts` | Inventory hosts | `Enter`/`d`: groups, variables and recent runs of the host |
| `:groups` | Inventory groups | `Enter`: hosts of the group, subgroups included |
| `:playbooks` | YAML files of `./playbook` (or `-p <dir>`; falls back to the project root if `./playbook` is missing) | `Enter`: hosts and tasks the playbook targets · `r`: run |
| `:runs` | Run history of the project | `Enter`: open · `r`: rerun · `f`: rerun on failed hosts · `ctrl+d`: delete |

## Keys

| Key | Action |
|---|---|
| `:` | Switch to a view (`Tab` completes, `:q` quits) |
| `/` | Filter the current view |
| `Enter` / `Esc` | Open / go back (`Esc` clears the filter first) |
| `↑↓` `j k`, `g G` | Move, go to top/bottom |
| `ctrl+r` | Refresh |
| `?` | Help for the current view |
| `ctrl+c` | Interrupt the current run, or quit |

See also: [Running a playbook](running-playbooks.md), [Command line](cli.md), [Run history](history.md).
