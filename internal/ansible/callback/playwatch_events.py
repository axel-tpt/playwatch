# Embedded in the playwatch binary and written to a temporary directory at run time.
# It streams one JSON object per line to the file descriptor named by
# PLAYWATCH_EVENTS_FD. The event contract is mirrored in internal/ansible/events.go:
# any change must bump EVENTS_VERSION on both sides.

from __future__ import annotations

import datetime
import json
import os

from ansible.plugins.callback import CallbackBase

try:
    from ansible.module_utils.common.json import AnsibleJSONEncoder
except ImportError:  # pragma: no cover - very old Ansible
    AnsibleJSONEncoder = json.JSONEncoder

DOCUMENTATION = """
    name: playwatch_events
    type: notification
    short_description: Stream structured run events to playwatch
    description:
      - Writes one JSON event per line to the file descriptor in PLAYWATCH_EVENTS_FD.
"""

EVENTS_VERSION = 1
CENSORED = "the output has been hidden due to the fact that 'no_log: true' was specified for this result"


class _Encoder(AnsibleJSONEncoder):
    def default(self, o):
        try:
            return super().default(o)
        except TypeError:
            return str(o)


def _censor(res):
    """Drop internal keys and hide results flagged with no_log."""
    if not isinstance(res, dict):
        return res
    if res.get("_ansible_no_log"):
        return {"censored": CENSORED, "changed": bool(res.get("changed", False))}
    clean = {k: v for k, v in res.items() if not k.startswith("_ansible_")}
    if isinstance(clean.get("results"), list):
        clean["results"] = [_censor(r) for r in clean["results"]]
    return clean


class CallbackModule(CallbackBase):
    CALLBACK_VERSION = 2.0
    CALLBACK_TYPE = "notification"
    CALLBACK_NAME = "playwatch_events"
    CALLBACK_NEEDS_ENABLED = True

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self._out = None
        self._play = None
        self._play_uuid = None
        fd = os.environ.get("PLAYWATCH_EVENTS_FD")
        if fd:
            try:
                # Commands exec'd by Ansible must not hold the pipe open, or
                # playwatch would never see the end of the stream.
                os.set_inheritable(int(fd), False)
                self._out = os.fdopen(int(fd), "w", buffering=1, encoding="utf-8")
            except (OSError, ValueError) as e:
                self._display.warning("playwatch_events: cannot open fd %s: %s" % (fd, e))

    def _emit(self, type_, **fields):
        if self._out is None:
            return
        event = {
            "v": EVENTS_VERSION,
            "type": type_,
            "ts": datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="milliseconds"),
        }
        event.update({k: v for k, v in fields.items() if v is not None})
        try:
            self._out.write(json.dumps(event, cls=_Encoder, separators=(",", ":")) + "\n")
        except (OSError, ValueError):
            # The reader went away: stop emitting, never break the run.
            self._out = None

    def _task_fields(self, task):
        return {
            "play": self._play,
            "play_uuid": self._play_uuid,
            "task": task.get_name(),
            "task_uuid": task._uuid,
            "task_action": task.action,
        }

    def _result_event(self, type_, result, **extra):
        res = result._result
        self._emit(
            type_,
            host=result._host.get_name(),
            changed=bool(res.get("changed", False)),
            result=_censor(res),
            **self._task_fields(result._task),
            **extra,
        )

    def v2_playbook_on_start(self, playbook):
        self._emit("playbook_on_start", playbook=playbook._file_name)

    def v2_playbook_on_play_start(self, play):
        self._play = play.get_name()
        self._play_uuid = play._uuid
        self._emit("playbook_on_play_start", play=self._play, play_uuid=self._play_uuid)

    def v2_playbook_on_task_start(self, task, is_conditional):
        self._emit("playbook_on_task_start", **self._task_fields(task))

    def v2_playbook_on_handler_task_start(self, task):
        self._emit("playbook_on_task_start", is_handler=True, **self._task_fields(task))

    def v2_runner_on_start(self, host, task):
        self._emit("runner_on_start", host=host.get_name(), **self._task_fields(task))

    def v2_runner_on_ok(self, result):
        self._result_event("runner_on_ok", result)

    def v2_runner_on_failed(self, result, ignore_errors=False):
        self._result_event("runner_on_failed", result, ignore_errors=bool(ignore_errors) or None)

    def v2_runner_on_skipped(self, result):
        self._result_event("runner_on_skipped", result)

    def v2_runner_on_unreachable(self, result):
        self._result_event("runner_on_unreachable", result)

    def v2_runner_item_on_ok(self, result):
        self._result_event("runner_item_on_ok", result)

    def v2_runner_item_on_failed(self, result):
        self._result_event("runner_item_on_failed", result)

    def v2_runner_item_on_skipped(self, result):
        self._result_event("runner_item_on_skipped", result)

    def v2_playbook_on_stats(self, stats):
        summary = {h: stats.summarize(h) for h in sorted(stats.processed)}
        self._emit("playbook_on_stats", stats=summary)
        if self._out is not None:
            self._out.close()
            self._out = None
