package ansible

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

func loadEventsFixture(t *testing.T) []Event {
	t.Helper()
	data, err := os.ReadFile("testdata/events_site.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	var events []Event
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		e, err := ParseEvent(sc.Bytes())
		if err != nil {
			t.Fatalf("ParseEvent(%s): %v", sc.Bytes(), err)
		}
		events = append(events, e)
	}
	return events
}

func TestParseEventsFixture(t *testing.T) {
	events := loadEventsFixture(t)
	if len(events) != 49 {
		t.Fatalf("got %d events, want 49", len(events))
	}

	first, last := events[0], events[len(events)-1]
	if first.Type != EventPlaybookStart || first.Playbook != "site.yml" {
		t.Errorf("first event = %+v, want playbook_on_start site.yml", first)
	}
	if first.TS.IsZero() {
		t.Error("timestamp not parsed")
	}
	if last.Type != EventStats {
		t.Fatalf("last event type = %s, want stats", last.Type)
	}
	want := HostStats{OK: 5, Changed: 1, Failures: 1, Skipped: 1, Ignored: 1}
	if got := last.Stats["web2"]; got != want {
		t.Errorf("web2 stats = %+v, want %+v", got, want)
	}

	// Final status per (task, host) of the Web tier play.
	status := map[[2]string]Status{}
	var handlers int
	for _, e := range events {
		if e.Type == EventTaskStart && e.IsHandler {
			handlers++
		}
		if s := e.Status(); s != StatusNone {
			status[[2]string{e.Task, e.Host}] = s
		}
	}
	if handlers != 1 {
		t.Errorf("got %d handler task starts, want 1", handlers)
	}
	tests := map[[2]string]Status{
		{"Pretend to reconfigure", "web1"}: StatusChanged,
		{"Only on web1", "web2"}:           StatusSkipped,
		{"Tolerated failure", "web1"}:      StatusIgnored,
		{"Fail on web2", "web2"}:           StatusFailed,
		{"Pretend to reload", "web1"}:      StatusOK,
		{"Show role", "db1"}:               StatusOK,
	}
	for key, want := range tests {
		if got := status[key]; got != want {
			t.Errorf("status%v = %q, want %q", key, got, want)
		}
	}
}

func TestEventsNoLogCensored(t *testing.T) {
	for _, e := range loadEventsFixture(t) {
		if e.Task != "Secret task" || e.Type != EventRunnerOK {
			continue
		}
		var res map[string]any
		if err := json.Unmarshal(e.Result, &res); err != nil {
			t.Fatal(err)
		}
		if _, ok := res["censored"]; !ok || len(res) != 2 {
			t.Errorf("no_log result not censored: %s", e.Result)
		}
		if bytes.Contains(e.Result, []byte("hunter2")) {
			t.Errorf("secret leaked: %s", e.Result)
		}
	}
}

func TestParseEventInvalid(t *testing.T) {
	for _, line := range []string{
		`not json`,
		`{"v":2,"type":"runner_on_ok"}`,
		`{"v":1}`,
	} {
		if _, err := ParseEvent([]byte(line)); err == nil {
			t.Errorf("ParseEvent(%s) succeeded, want error", line)
		}
	}
}
