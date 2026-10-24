package ansible

import (
	"os"
	"slices"
	"testing"
)

func TestParseRolesPath(t *testing.T) {
	data, err := os.ReadFile("testdata/config_dump.json")
	if err != nil {
		t.Fatal(err)
	}
	got, err := parseRolesPath(data)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(got, "/usr/share/ansible/roles") || len(got) < 2 {
		t.Errorf("got %v", got)
	}
	if _, err := parseRolesPath([]byte("nope")); err == nil {
		t.Error("invalid JSON: want error")
	}
	if _, err := parseRolesPath([]byte(`[{"name":"X","value":1}]`)); err == nil {
		t.Error("missing entry: want error")
	}
}
