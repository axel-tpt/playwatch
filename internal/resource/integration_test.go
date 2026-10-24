//go:build integration

package resource

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/axel-tpt/playwatch/internal/ansible"
)

func TestIntegrationRoles(t *testing.T) {
	dir, err := filepath.Abs("../../testdata/project")
	if err != nil {
		t.Fatal(err)
	}
	client := ansible.NewClient("inventory.ini")
	client.Dir = dir
	res := Roles(Env{Ctx: context.Background(), Client: client, Dir: dir})
	rows, err := res.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Cells[0] == "demo" && r.Cells[1] == "main.yml" {
			return
		}
	}
	t.Errorf("no demo row in %v", rows)
}
