package store

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/axel-tpt/playwatch/internal/ansible"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "runs"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestRecordAndReadBack(t *testing.T) {
	s := openTemp(t)
	rec, err := s.Create(Meta{Playbook: "site.yml", Project: "/p", Args: []string{"--limit", "web"}})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	id := rec.Meta().ID

	running, err := s.Meta(id)
	if err != nil || running.State != StateRunning || running.ExitCode != -1 {
		t.Fatalf("Meta while running = %+v, %v", running, err)
	}

	events := []ansible.Event{
		{V: 1, Type: ansible.EventPlaybookStart, Playbook: "site.yml"},
		{V: 1, Type: ansible.EventRunnerOK, Host: "web1", TaskUUID: "t1", Result: json.RawMessage(`{"msg":"hi","n":12345678901234567890}`)},
	}
	for _, e := range events {
		if err := rec.Record(e); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := io.WriteString(rec.Output(), "PLAY [x]\n"); err != nil {
		t.Fatal(err)
	}
	stats := map[string]ansible.HostStats{"web1": {OK: 1}}
	if err := rec.Finish(StateFailed, 2, stats); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	meta, err := s.Meta(id)
	if err != nil {
		t.Fatal(err)
	}
	if meta.State != StateFailed || meta.ExitCode != 2 || meta.Stats["web1"].OK != 1 || meta.Finished.IsZero() {
		t.Errorf("final meta = %+v", meta)
	}
	got, err := s.Events(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || string(got[1].Result) != `{"msg":"hi","n":12345678901234567890}` {
		t.Errorf("events read back = %+v", got)
	}
	out, err := s.Output(id)
	if err != nil || out != "PLAY [x]\n" {
		t.Errorf("output = %q, %v", out, err)
	}

	info, err := os.Stat(filepath.Join(s.Dir(), id+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("events file mode = %o, want 600", perm)
	}
}

func TestListFiltersAndSorts(t *testing.T) {
	s := openTemp(t)
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	for i, project := range []string{"/a", "/b", "/a"} {
		rec, err := s.Create(Meta{Playbook: "site.yml", Project: project, Started: base.Add(time.Duration(i) * time.Minute)})
		if err != nil {
			t.Fatal(err)
		}
		if err := rec.Finish(StateOK, 0, nil); err != nil {
			t.Fatal(err)
		}
	}
	// A foreign file must not break the listing.
	if err := os.WriteFile(filepath.Join(s.Dir(), "junk.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}

	metas, err := s.List("/a")
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 2 || !metas[0].Started.After(metas[1].Started) {
		t.Errorf("List(/a) = %+v, want 2 runs newest first", metas)
	}
	all, _ := s.List("")
	if len(all) != 3 {
		t.Errorf("List(\"\") = %d runs, want 3", len(all))
	}
}

func TestDeadRunIsAborted(t *testing.T) {
	s := openTemp(t)
	rec, err := s.Create(Meta{Playbook: "site.yml"})
	if err != nil {
		t.Fatal(err)
	}
	meta := rec.Meta()
	meta.PID = 1 << 30 // no such process
	if err := s.writeMeta(meta); err != nil {
		t.Fatal(err)
	}
	got, err := s.Meta(meta.ID)
	if err != nil || got.State != StateAborted {
		t.Errorf("state = %q (%v), want aborted", got.State, err)
	}
}

func TestEventsIgnoresTruncatedLine(t *testing.T) {
	s := openTemp(t)
	rec, err := s.Create(Meta{Playbook: "site.yml"})
	if err != nil {
		t.Fatal(err)
	}
	if err := rec.Record(ansible.Event{V: 1, Type: ansible.EventPlaybookStart}); err != nil {
		t.Fatal(err)
	}
	if _, err := rec.events.WriteString(`{"v":1,"type":"runner_o`); err != nil {
		t.Fatal(err)
	}
	events, err := s.Events(rec.Meta().ID)
	if err != nil || len(events) != 1 {
		t.Errorf("Events = %d events, %v; want 1", len(events), err)
	}
}

func TestPrune(t *testing.T) {
	s := openTemp(t)
	var ids []string
	for i := range 4 {
		rec, err := s.Create(Meta{Playbook: "p.yml", Started: time.Now().Add(time.Duration(i) * time.Minute)})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, rec.Meta().ID)
		if i < 3 {
			if err := rec.Finish(StateOK, 0, nil); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := s.Prune(1); err != nil {
		t.Fatal(err)
	}
	metas, _ := s.List("")
	if len(metas) != 2 {
		t.Fatalf("after Prune(1): %d runs, want the running one and the newest finished one", len(metas))
	}
	if metas[0].ID != ids[3] || metas[1].ID != ids[2] {
		t.Errorf("kept %s, %s; want %s, %s", metas[0].ID, metas[1].ID, ids[3], ids[2])
	}
	if _, err := os.Stat(filepath.Join(s.Dir(), ids[0]+".jsonl")); !os.IsNotExist(err) {
		t.Error("pruned run files still exist")
	}
}

func TestDefaultDir(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/xdg")
	if dir, _ := DefaultDir(); dir != "/xdg/playwatch/runs" {
		t.Errorf("DefaultDir() = %q", dir)
	}
}

func TestMetaRunOptions(t *testing.T) {
	legacy := Meta{Args: []string{"--limit", "web"}}
	if got := legacy.RunOptions(); len(got.Extra) != 2 || got.Limit != "" {
		t.Errorf("legacy run options = %+v, want raw args in Extra", got)
	}
	opts := ansible.RunOptions{Limit: "web", Check: true}
	if got := (Meta{Options: &opts, Args: opts.Args()}).RunOptions(); got.Limit != "web" || !got.Check {
		t.Errorf("run options = %+v", got)
	}
}
