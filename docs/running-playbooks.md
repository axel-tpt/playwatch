# Running a playbook

Press `r` on a playbook. A form opens, prefilled with the options of that playbook's last run:

- limit (`Tab` completes host and group names)
- tags and skip tags
- extra vars
- check mode and diff
- a vault password or a vault password file

The exact `ansible-playbook` command is shown as you type. Press `Enter` to run it.

## The run view

The run view fills in live as Ansible works:

- `←→` and `↑↓` move between cells.
- `Enter` on a cell shows the full JSON result of that task on that host. On the task column, it shows the result for every host.
- `o` shows Ansible's raw output.
- `ctrl+c` interrupts the run cleanly.
- `Esc` leaves the run going in the background. Get back to it from `:runs`.

## Vault

A vault password typed into the form is handed to Ansible through a pipe. It is never written to disk or saved in the history.
