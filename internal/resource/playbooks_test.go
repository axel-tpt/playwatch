package resource

import (
	"os"
	"path/filepath"
	"testing"
)

func ids(t *testing.T, project, pbDir string) []string {
	t.Helper()
	rows, err := listPlaybooks(project, pbDir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range rows {
		out = append(out, r.ID)
	}
	return out
}

func TestListPlaybooks(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{"playbook/a.yml", "other/b.yaml", "top.yml"} {
		p := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got := ids(t, root, ""); len(got) != 1 || got[0] != filepath.Join("playbook", "a.yml") {
		t.Errorf("default: %v", got)
	}
	if got := ids(t, root, "other"); len(got) != 1 || got[0] != filepath.Join("other", "b.yaml") {
		t.Errorf("custom: %v", got)
	}
	if got := ids(t, root, filepath.Join(root, "other")); len(got) != 1 || got[0] != filepath.Join("other", "b.yaml") {
		t.Errorf("absolute: %v", got)
	}
	if err := os.RemoveAll(filepath.Join(root, "playbook")); err != nil {
		t.Fatal(err)
	}
	if got := ids(t, root, ""); len(got) != 1 || got[0] != "top.yml" {
		t.Errorf("fallback: %v", got)
	}
}
