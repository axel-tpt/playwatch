package resource

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestRoleSearchDirs(t *testing.T) {
	got := roleSearchDirs("/p", "/p/playbook", []string{"/p/roles", "/etc/r", "/etc/r"})
	want := []string{"/p/playbook/roles", "/p/roles", "/etc/r"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func mk(t *testing.T, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if strings.HasSuffix(p, "/") {
			if err := os.MkdirAll(p, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestListRoles(t *testing.T) {
	root := t.TempDir()
	other := t.TempDir()
	mk(t,
		root+"/roles/web/tasks/main.yml",
		root+"/roles/db/tasks/main.yaml",
		root+"/roles/empty/",
		root+"/roles/.hidden/tasks/main.yml",
		root+"/roles/file.txt",
		other+"/web/tasks/main.yml",
	)
	rows, err := listRoles(root, []string{root + "/missing", root + "/roles", other})
	if err != nil {
		t.Fatal(err)
	}
	type r struct{ name, tasks, path string }
	var got []r
	for _, row := range rows {
		got = append(got, r{row.Cells[0], row.Cells[1], row.Cells[2]})
	}
	want := []r{
		{"db", "main.yaml", "roles"},
		{"empty", "-", "roles"},
		{"web", "main.yml", "roles"},
		{"web", "main.yml", other},
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if rows[0].ID != filepath.Join(root, "roles", "db") {
		t.Errorf("ID = %q", rows[0].ID)
	}
}

func TestRoleTasksFile(t *testing.T) {
	root := t.TempDir()
	mk(t, root+"/a/tasks/main.yml", root+"/a/tasks/main.yaml", root+"/b/tasks/main.yaml", root+"/c/")
	if f, err := roleTasksFile(root + "/a"); err != nil || filepath.Base(f) != "main.yml" {
		t.Errorf("a: %q %v", f, err)
	}
	if f, err := roleTasksFile(root + "/b"); err != nil || filepath.Base(f) != "main.yaml" {
		t.Errorf("b: %q %v", f, err)
	}
	if _, err := roleTasksFile(root + "/c"); err == nil || !strings.Contains(err.Error(), "role c") {
		t.Errorf("c: %v", err)
	}
}
