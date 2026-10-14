# Command line

The same features are available without the TUI:

```bash
playwatch -i inventory.ini hosts            # list hosts
playwatch -i inventory.ini groups           # list groups
playwatch -i inventory.ini host web1        # variables of a host, as JSON
playwatch -i inventory.ini run playbook/site.yml -l web -C -D -e "version=2"
playwatch runs                              # run history of the current project
```

`playwatch run` accepts the usual `ansible-playbook` options: `-l`, `-t`, `--skip-tags`, `-C`, `-D`, `-e`, `--vault-password-file` and `--vault-id`. It prints one line per task result (`--json` for raw events, `--raw` for Ansible's own output) and exits with Ansible's exit code.
