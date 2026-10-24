//go:build integration

package ansible

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

// These tests need Ansible in $PATH and only target testdata/project, whose
// hosts are all aliases of localhost.

func projectClient() *Client {
	c := NewClient("inventory.ini")
	c.Dir = "../../testdata/project"
	return c
}

func TestIntegrationInventory(t *testing.T) {
	inv, err := projectClient().Inventory(context.Background())
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}
	if got := inv.GroupHosts("prod"); len(got) != 3 {
		t.Errorf("GroupHosts(prod) = %v, want 3 hosts", got)
	}
	if inv.Warnings != "" {
		t.Errorf("unexpected warnings: %s", inv.Warnings)
	}
}

func TestIntegrationHostVars(t *testing.T) {
	vars, err := projectClient().HostVars(context.Background(), "db1")
	if err != nil {
		t.Fatalf("HostVars: %v", err)
	}
	if got := vars["db_port"]; got != json.Number("5432") {
		t.Errorf("db_port = %#v, want 5432", got)
	}
}

func TestIntegrationUnknownHost(t *testing.T) {
	_, err := projectClient().HostVars(context.Background(), "nope")
	var cmdErr *CommandError
	if !errors.As(err, &cmdErr) || cmdErr.Stderr == "" {
		t.Errorf("err = %v, want *CommandError with Ansible's stderr", err)
	}
}

func TestIntegrationRunPlaybook(t *testing.T) {
	run, err := projectClient().RunPlaybook(context.Background(), PlaybookOptions{Playbook: "playbook/site.yml"})
	if err != nil {
		t.Fatalf("RunPlaybook: %v", err)
	}
	var events []Event
	for e := range run.Events {
		events = append(events, e)
	}
	err = run.Wait()

	// web2 fails on purpose: ansible-playbook exits with 2.
	if code := run.ExitCode(); code != 2 {
		t.Errorf("ExitCode() = %d, want 2 (err %v)", code, err)
	}
	if len(events) == 0 || events[0].Type != EventPlaybookStart || events[len(events)-1].Type != EventStats {
		t.Fatalf("unexpected event stream: %d events", len(events))
	}
	for _, e := range events {
		if strings.Contains(string(e.Result), "hunter2") {
			t.Errorf("no_log secret leaked in %s event", e.Type)
		}
	}
	if got := events[len(events)-1].Stats["web2"].Failures; got != 1 {
		t.Errorf("web2 failures = %d, want 1", got)
	}
}

func TestIntegrationRunPlaybookCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	run, err := projectClient().RunPlaybook(ctx, PlaybookOptions{Playbook: "playbook/slow.yml"})
	if err != nil {
		t.Fatalf("RunPlaybook: %v", err)
	}
	start := time.Now()
	for e := range run.Events {
		if e.Type == EventRunnerStart {
			cancel()
		}
	}
	if err := run.Wait(); err == nil {
		t.Error("Wait() = nil, want an error after cancellation")
	}
	if elapsed := time.Since(start); elapsed > 15*time.Second {
		t.Errorf("run not interrupted promptly (took %v)", elapsed)
	}
}

func TestIntegrationVaultPassword(t *testing.T) {
	runVault := func(password string) (*Run, []Event) {
		run, err := projectClient().RunPlaybook(context.Background(), PlaybookOptions{
			Playbook: "playbook/vault.yml",
			VaultPassword: []byte(password),
		})
		if err != nil {
			t.Fatalf("RunPlaybook: %v", err)
		}
		var events []Event
		for e := range run.Events {
			events = append(events, e)
		}
		_ = run.Wait()
		return run, events
	}

	run, events := runVault("a5e-test")
	if code := run.ExitCode(); code != 0 {
		t.Fatalf("with the right password: exit code %d", code)
	}
	found := false
	for _, e := range events {
		if e.Type == EventRunnerOK && strings.Contains(string(e.Result), "secret has 18 characters") {
			found = true
		}
	}
	if !found {
		t.Error("the decrypted secret was not used")
	}

	if run, _ := runVault("wrong"); run.ExitCode() == 0 {
		t.Error("a wrong vault password should fail the run")
	}
}

func TestIntegrationRolesPath(t *testing.T) {
	paths, err := projectClient().RolesPath(context.Background())
	if err != nil {
		t.Fatalf("RolesPath: %v", err)
	}
	if len(paths) == 0 {
		t.Error("empty roles path")
	}
}
