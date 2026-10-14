# Run history

Runs are saved under `$XDG_STATE_HOME/playwatch/runs` (by default `~/.local/state/playwatch/runs`), readable only by you. Each run keeps its options, its events and Ansible's raw output. The 200 most recent runs are kept.

From `:runs` you can open a past run, rerun it (`r`), rerun it only on the hosts that failed (`f`), or delete it (`ctrl+d`).
