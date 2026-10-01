package ansible

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// fakePlaybook installs a fake ansible-playbook as the only binary in $PATH.
// It records its arguments, environment and the callback dir listing next to
// itself, then runs body.
func fakePlaybook(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	script := `#!/bin/sh
here="$(dirname "$0")"
printf '%s\n' "$@" > "$here/args.txt"
env > "$here/env.txt"
ls "$ANSIBLE_CALLBACK_PLUGINS" > "$here/plugins.txt"
` + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, "ansible-playbook"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":/bin:/usr/bin")
	return dir
}

func collect(t *testing.T, run *Run) []Event {
	t.Helper()
	var events []Event
	timeout := time.After(10 * time.Second)
	for {
		select {
		case e, ok := <-run.Events:
			if !ok {
				return events
			}
			events = append(events, e)
		case <-timeout:
			t.Fatal("events channel not closed in time")
		}
	}
}

func TestRunPlaybookStreamsEvents(t *testing.T) {
	fixture, err := filepath.Abs("testdata/events_site.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	dir := fakePlaybook(t, `echo "PLAY [Web tier]"; echo warn >&2; cat '`+fixture+`' >&3; exit 2`)

	var out bytes.Buffer
	c := NewClient("inventory.ini")
	run, err := c.RunPlaybook(context.Background(), PlaybookOptions{
		Playbook: "site.yml",
		Args:     []string{"--limit", "web"},
		Output:   &out,
	})
	if err != nil {
		t.Fatalf("RunPlaybook: %v", err)
	}
	events := collect(t, run)
	err = run.Wait()

	if len(events) != 49 {
		t.Errorf("got %d events, want 49", len(events))
	}
	var cmdErr *CommandError
	if !errors.As(err, &cmdErr) || !strings.Contains(cmdErr.Stderr, "warn") {
		t.Errorf("Wait() = %v, want *CommandError with stderr", err)
	}
	if code := run.ExitCode(); code != 2 {
		t.Errorf("ExitCode() = %d, want 2", code)
	}
	if !strings.Contains(out.String(), "PLAY [Web tier]") || !strings.Contains(out.String(), "warn") {
		t.Errorf("Output = %q, want stdout and stderr", out.String())
	}

	args, _ := os.ReadFile(filepath.Join(dir, "args.txt"))
	want := []string{"-i", "inventory.ini", "--limit", "web", "site.yml"}
	if got := strings.Fields(string(args)); !slices.Equal(got, want) {
		t.Errorf("args = %v, want %v", got, want)
	}
	env, _ := os.ReadFile(filepath.Join(dir, "env.txt"))
	for _, kv := range []string{"ANSIBLE_CALLBACKS_ENABLED=playwatch_events", "PLAYWATCH_EVENTS_FD=3"} {
		if !strings.Contains(string(env), kv) {
			t.Errorf("env lacks %s", kv)
		}
	}
	plugins, _ := os.ReadFile(filepath.Join(dir, "plugins.txt"))
	if strings.TrimSpace(string(plugins)) != "playwatch_events.py" {
		t.Errorf("callback dir contains %q, want playwatch_events.py", plugins)
	}
}

func TestRunPlaybookCleansUpPluginDir(t *testing.T) {
	dir := fakePlaybook(t, `echo "$ANSIBLE_CALLBACK_PLUGINS" > "$here/plugindir.txt"`)
	run, err := NewClient().RunPlaybook(context.Background(), PlaybookOptions{Playbook: "site.yml"})
	if err != nil {
		t.Fatal(err)
	}
	collect(t, run)
	if err := run.Wait(); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	pluginDir, _ := os.ReadFile(filepath.Join(dir, "plugindir.txt"))
	if _, err := os.Stat(strings.TrimSpace(string(pluginDir))); !os.IsNotExist(err) {
		t.Errorf("plugin dir %s not removed (stat err %v)", pluginDir, err)
	}
}

func TestRunPlaybookMalformedEvent(t *testing.T) {
	fakePlaybook(t, `echo 'garbage' >&3; echo '{"v":1,"type":"playbook_on_start","ts":"2026-01-01T00:00:00Z"}' >&3`)
	run, err := NewClient().RunPlaybook(context.Background(), PlaybookOptions{Playbook: "site.yml"})
	if err != nil {
		t.Fatal(err)
	}
	if events := collect(t, run); len(events) != 1 {
		t.Errorf("got %d events, want the valid one", len(events))
	}
	if err := run.Wait(); err == nil || !strings.Contains(err.Error(), "parse event") {
		t.Errorf("Wait() = %v, want parse error", err)
	}
}

func TestRunPlaybookCancel(t *testing.T) {
	fakePlaybook(t, `exec sleep 30`)
	ctx, cancel := context.WithCancel(context.Background())
	run, err := NewClient().RunPlaybook(ctx, PlaybookOptions{Playbook: "site.yml"})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	cancel()
	collect(t, run)
	if err := run.Wait(); err == nil {
		t.Error("Wait() = nil, want an error after cancellation")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("run not interrupted promptly (took %v)", elapsed)
	}
}

func TestRunPlaybookVaultPassword(t *testing.T) {
	// The fake reads the password from the file named by --vault-password-file.
	dir := fakePlaybook(t, `while [ $# -gt 0 ]; do
  if [ "$1" = --vault-password-file ]; then cat "$2" > "$here/vault.txt"; fi
  shift
done`)
	password := []byte("s3cret")
	run, err := NewClient().RunPlaybook(context.Background(), PlaybookOptions{
		Playbook:      "vault.yml",
		Args:          []string{"--check"},
		VaultPassword: password,
	})
	if err != nil {
		t.Fatal(err)
	}
	clear(password) // callers wipe their copy right away
	collect(t, run)
	if err := run.Wait(); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	args, _ := os.ReadFile(filepath.Join(dir, "args.txt"))
	want := []string{"--check", "--vault-password-file", "/dev/fd/4", "vault.yml"}
	if got := strings.Fields(string(args)); !slices.Equal(got, want) {
		t.Errorf("args = %v, want %v", got, want)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "vault.txt")); string(got) != "s3cret" {
		t.Errorf("password read by ansible = %q, want s3cret", got)
	}
}
