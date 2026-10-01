package ansible

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// fakeAnsible installs a fake ansible-inventory shell script as the only
// binary in $PATH. The script records its arguments in args.txt next to it.
func fakeAnsible(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$(dirname \"$0\")/args.txt\"\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, "ansible-inventory"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":/bin:/usr/bin")
	return dir
}

func TestClientInventoryWithFakeBinary(t *testing.T) {
	fixture, err := filepath.Abs("testdata/inventory_list.json")
	if err != nil {
		t.Fatal(err)
	}
	dir := fakeAnsible(t, `cat '`+fixture+`'; echo '[WARNING]: something odd' >&2`)

	c := NewClient("a.ini", "b.yml")
	inv, err := c.Inventory(context.Background())
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}
	if len(inv.Hosts) != 5 {
		t.Errorf("got %d hosts, want 5", len(inv.Hosts))
	}
	if !strings.Contains(inv.Warnings, "something odd") {
		t.Errorf("Warnings = %q, want stderr kept on success", inv.Warnings)
	}

	args, err := os.ReadFile(filepath.Join(dir, "args.txt"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-i", "a.ini", "-i", "b.yml", "--list"}
	if got := strings.Fields(string(args)); !slices.Equal(got, want) {
		t.Errorf("args = %v, want %v", got, want)
	}
}

func TestClientCommandError(t *testing.T) {
	fakeAnsible(t, `echo '[ERROR]: You must pass a single valid host' >&2; exit 1`)

	_, err := NewClient().HostVars(context.Background(), "nope")
	var cmdErr *CommandError
	if !errors.As(err, &cmdErr) {
		t.Fatalf("err = %v, want *CommandError", err)
	}
	if !strings.Contains(cmdErr.Stderr, "You must pass a single valid host") {
		t.Errorf("Stderr = %q, want Ansible's message verbatim", cmdErr.Stderr)
	}
	if !strings.Contains(err.Error(), "You must pass a single valid host") {
		t.Errorf("Error() = %q, want it to include stderr", err)
	}
}

func TestClientTimeout(t *testing.T) {
	fakeAnsible(t, `exec sleep 10`)

	c := NewClient()
	c.Timeout = 100 * time.Millisecond
	start := time.Now()
	_, err := c.Inventory(context.Background())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want context.DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("command was not killed promptly (took %v)", elapsed)
	}
}

func TestClientNotInstalled(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := NewClient().Inventory(context.Background())
	if !errors.Is(err, ErrNotInstalled) {
		t.Errorf("err = %v, want ErrNotInstalled", err)
	}
}
