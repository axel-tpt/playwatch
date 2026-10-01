package ansible

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
)

func loadInventoryFixture(t *testing.T) *Inventory {
	t.Helper()
	data, err := os.ReadFile("testdata/inventory_list.json")
	if err != nil {
		t.Fatal(err)
	}
	inv, err := ParseInventory(data)
	if err != nil {
		t.Fatalf("ParseInventory: %v", err)
	}
	return inv
}

func TestParseInventory(t *testing.T) {
	inv := loadInventoryFixture(t)

	wantHosts := []string{"bastion", "db1", "tools", "web1", "web2"}
	if got := inv.HostNames(); !slices.Equal(got, wantHosts) {
		t.Errorf("HostNames() = %v, want %v", got, wantHosts)
	}
	wantGroups := []string{"all", "db", "prod", "standalone", "ungrouped", "web"}
	if got := inv.GroupNames(); !slices.Equal(got, wantGroups) {
		t.Errorf("GroupNames() = %v, want %v", got, wantGroups)
	}

	web1 := inv.Hosts["web1"]
	if !slices.Equal(web1.Groups, []string{"web"}) {
		t.Errorf("web1.Groups = %v, want [web]", web1.Groups)
	}
	if got := web1.Vars["http_port"]; got != json.Number("8080") {
		t.Errorf("web1 http_port = %#v, want json.Number(8080)", got)
	}
	if got := web1.Vars["env"]; got != "production" {
		t.Errorf("web1 env = %#v, want production (from group_vars/prod.yml)", got)
	}
	if got := inv.Hosts["bastion"].Groups; !slices.Equal(got, []string{"ungrouped"}) {
		t.Errorf("bastion.Groups = %v, want [ungrouped]", got)
	}
	if got := inv.Groups["prod"].Children; !slices.Equal(got, []string{"web", "db"}) {
		t.Errorf("prod.Children = %v, want [web db]", got)
	}
}

func TestInventoryGroupHosts(t *testing.T) {
	inv := loadInventoryFixture(t)
	tests := map[string][]string{
		"web":     {"web1", "web2"},
		"prod":    {"db1", "web1", "web2"},
		"all":     {"bastion", "db1", "tools", "web1", "web2"},
		"missing": nil,
	}
	for group, want := range tests {
		if got := inv.GroupHosts(group); !slices.Equal(got, want) {
			t.Errorf("GroupHosts(%q) = %v, want %v", group, got, want)
		}
	}
}

func TestInventoryHostGroups(t *testing.T) {
	inv := loadInventoryFixture(t)
	tests := map[string][]string{
		"web1":    {"all", "prod", "web"},
		"bastion": {"all", "ungrouped"},
		"missing": nil,
	}
	for host, want := range tests {
		if got := inv.HostGroups(host); !slices.Equal(got, want) {
			t.Errorf("HostGroups(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestParseInventoryEdgeCases(t *testing.T) {
	// Group vars (as printed with --export), an empty child group without an
	// entry of its own, a host without hostvars and a group cycle.
	data := []byte(`{
		"_meta": {"hostvars": {}},
		"all": {"children": ["ungrouped", "a"]},
		"a": {"hosts": ["h1"], "children": ["b"], "vars": {"x": 1}},
		"b": {"hosts": ["h1", "h2"], "children": ["a"]}
	}`)
	inv, err := ParseInventory(data)
	if err != nil {
		t.Fatalf("ParseInventory: %v", err)
	}
	if _, ok := inv.Groups["ungrouped"]; !ok {
		t.Error("empty child group ungrouped was not created")
	}
	if got := inv.Groups["a"].Vars["x"]; got != json.Number("1") {
		t.Errorf("a.Vars[x] = %#v, want 1", got)
	}
	if got := inv.Hosts["h1"].Groups; !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("h1.Groups = %v, want [a b]", got)
	}
	if got := inv.GroupHosts("a"); !slices.Equal(got, []string{"h1", "h2"}) {
		t.Errorf("GroupHosts(a) = %v, want [h1 h2]", got)
	}
	if got := inv.HostGroups("h2"); !slices.Equal(got, []string{"a", "all", "b"}) {
		t.Errorf("HostGroups(h2) = %v, want [a all b]", got)
	}
}

func TestParseInventoryInvalid(t *testing.T) {
	for _, data := range []string{``, `[]`, `{"web": "nope"}`} {
		if _, err := ParseInventory([]byte(data)); err == nil {
			t.Errorf("ParseInventory(%q) succeeded, want error", data)
		}
	}
}
