package ansible

import (
	"encoding/json"
	"fmt"
	"time"
)

// EventsVersion is the version of the event contract shared with the
// playwatch_events callback plugin (callback/playwatch_events.py). Any change to the
// contract bumps it on both sides.
const EventsVersion = 1

// EventType is the kind of an event, named after the callback hook that
// emitted it (without the v2_ prefix).
type EventType string

const (
	EventPlaybookStart     EventType = "playbook_on_start"
	EventPlayStart         EventType = "playbook_on_play_start"
	EventTaskStart         EventType = "playbook_on_task_start" // IsHandler set for handlers
	EventRunnerStart       EventType = "runner_on_start"
	EventRunnerOK          EventType = "runner_on_ok"
	EventRunnerFailed      EventType = "runner_on_failed"
	EventRunnerSkipped     EventType = "runner_on_skipped"
	EventRunnerUnreachable EventType = "runner_on_unreachable"
	EventItemOK            EventType = "runner_item_on_ok"
	EventItemFailed        EventType = "runner_item_on_failed"
	EventItemSkipped       EventType = "runner_item_on_skipped"
	EventStats             EventType = "playbook_on_stats"
)

// Event is one line emitted by the callback plugin. Fields irrelevant to a
// given type are left empty.
type Event struct {
	V    int       `json:"v"`
	Type EventType `json:"type"`
	TS   time.Time `json:"ts"`

	Playbook string `json:"playbook,omitempty"` // playbook_on_start

	Play       string `json:"play,omitempty"`
	PlayUUID   string `json:"play_uuid,omitempty"`
	Task       string `json:"task,omitempty"`
	TaskUUID   string `json:"task_uuid,omitempty"`
	TaskAction string `json:"task_action,omitempty"`
	IsHandler  bool   `json:"is_handler,omitempty"`

	Host         string `json:"host,omitempty"`
	Changed      bool   `json:"changed,omitempty"`
	IgnoreErrors bool   `json:"ignore_errors,omitempty"`
	// Result is the module result, already censored by the plugin when
	// no_log is set. It is kept raw for faithful display.
	Result json.RawMessage `json:"result,omitempty"`

	Stats map[string]HostStats `json:"stats,omitempty"` // playbook_on_stats
}

// HostStats is a host's line of the play recap.
type HostStats struct {
	OK          int `json:"ok"`
	Changed     int `json:"changed"`
	Failures    int `json:"failures"`
	Unreachable int `json:"unreachable"`
	Skipped     int `json:"skipped"`
	Rescued     int `json:"rescued"`
	Ignored     int `json:"ignored"`
}

// Status is the outcome of a task on a host, as shown to the user.
type Status string

const (
	StatusNone        Status = ""
	StatusRunning     Status = "running"
	StatusOK          Status = "ok"
	StatusChanged     Status = "changed"
	StatusSkipped     Status = "skipped"
	StatusFailed      Status = "failed"
	StatusIgnored     Status = "ignored" // failed with ignore_errors
	StatusUnreachable Status = "unreachable"
)

// Status returns the task/host status an event reports, or StatusNone for
// events that do not report one (play/task starts, loop items, stats).
func (e Event) Status() Status {
	switch e.Type {
	case EventRunnerStart:
		return StatusRunning
	case EventRunnerOK:
		if e.Changed {
			return StatusChanged
		}
		return StatusOK
	case EventRunnerSkipped:
		return StatusSkipped
	case EventRunnerFailed:
		if e.IgnoreErrors {
			return StatusIgnored
		}
		return StatusFailed
	case EventRunnerUnreachable:
		return StatusUnreachable
	}
	return StatusNone
}

// ParseEvent decodes one line of the event stream.
func ParseEvent(line []byte) (Event, error) {
	var e Event
	if err := json.Unmarshal(line, &e); err != nil {
		return Event{}, fmt.Errorf("parse event: %w", err)
	}
	if e.V != EventsVersion {
		return Event{}, fmt.Errorf("parse event: unsupported version %d (want %d)", e.V, EventsVersion)
	}
	if e.Type == "" {
		return Event{}, fmt.Errorf("parse event: missing type")
	}
	return e, nil
}
