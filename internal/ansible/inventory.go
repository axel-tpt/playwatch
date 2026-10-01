package ansible

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
)

// Inventory is the parsed output of `ansible-inventory --list`.
type Inventory struct {
	Groups map[string]*Group
	Hosts  map[string]*Host
	// Warnings is Ansible's stderr, kept verbatim. It can be non-empty even
	// on success, e.g. when an inventory source failed to parse.
	Warnings string
}

// Group is an inventory group with its direct members.
type Group struct {
	Name     string
	Hosts    []string // direct hosts only
	Children []string // direct child groups
	Vars     map[string]any
}

// Host is an inventory host.
type Host struct {
	Name string
	// Groups lists the groups the host belongs to directly, sorted.
	Groups []string
	// Vars are the host's variables as resolved by ansible-inventory
	// (inventory, group_vars and host_vars merged).
	Vars map[string]any
}

// Inventory runs `ansible-inventory --list` and parses its output.
func (c *Client) Inventory(ctx context.Context) (*Inventory, error) {
	args := append(c.inventoryArgs(), "--list")
	out, stderr, err := c.output(ctx, "ansible-inventory", args...)
	if err != nil {
		return nil, fmt.Errorf("list inventory: %w", err)
	}
	inv, err := ParseInventory(out)
	if err != nil {
		return nil, err
	}
	inv.Warnings = stderr
	return inv, nil
}

// HostVars runs `ansible-inventory --host` and returns the host's variables.
func (c *Client) HostVars(ctx context.Context, host string) (map[string]any, error) {
	args := append(c.inventoryArgs(), "--host", host)
	out, _, err := c.output(ctx, "ansible-inventory", args...)
	if err != nil {
		return nil, fmt.Errorf("vars of host %q: %w", host, err)
	}
	var vars map[string]any
	if err := decodeJSON(out, &vars); err != nil {
		return nil, fmt.Errorf("parse vars of host %q: %w", host, err)
	}
	return vars, nil
}

// ParseInventory parses the JSON printed by `ansible-inventory --list`.
func ParseInventory(data []byte) (*Inventory, error) {
	var raw map[string]json.RawMessage
	if err := decodeJSON(data, &raw); err != nil {
		return nil, fmt.Errorf("parse inventory: %w", err)
	}

	inv := &Inventory{Groups: map[string]*Group{}, Hosts: map[string]*Host{}}
	var meta struct {
		HostVars map[string]map[string]any `json:"hostvars"`
	}
	for name, msg := range raw {
		if name == "_meta" {
			if err := decodeJSON(msg, &meta); err != nil {
				return nil, fmt.Errorf("parse inventory _meta: %w", err)
			}
			continue
		}
		var g struct {
			Hosts    []string       `json:"hosts"`
			Children []string       `json:"children"`
			Vars     map[string]any `json:"vars"`
		}
		if err := decodeJSON(msg, &g); err != nil {
			return nil, fmt.Errorf("parse inventory group %q: %w", name, err)
		}
		inv.Groups[name] = &Group{Name: name, Hosts: g.Hosts, Children: g.Children, Vars: g.Vars}
	}

	// Empty groups (typically "ungrouped") are referenced as children
	// without having an entry of their own.
	for _, g := range inv.groupList() {
		for _, child := range g.Children {
			if _, ok := inv.Groups[child]; !ok {
				inv.Groups[child] = &Group{Name: child}
			}
		}
	}
	for _, g := range inv.Groups {
		for _, h := range g.Hosts {
			host := inv.host(h)
			host.Groups = append(host.Groups, g.Name)
		}
	}
	for name, vars := range meta.HostVars {
		inv.host(name).Vars = vars
	}
	for _, h := range inv.Hosts {
		slices.Sort(h.Groups)
		h.Groups = slices.Compact(h.Groups)
	}
	return inv, nil
}

// HostNames returns all host names, sorted.
func (inv *Inventory) HostNames() []string {
	return sortedKeys(inv.Hosts)
}

// GroupNames returns all group names, sorted.
func (inv *Inventory) GroupNames() []string {
	return sortedKeys(inv.Groups)
}

// GroupHosts returns the hosts of a group and of all its descendants, sorted.
func (inv *Inventory) GroupHosts(group string) []string {
	seen := map[string]bool{}
	var hosts []string
	var walk func(string)
	walk = func(name string) {
		g, ok := inv.Groups[name]
		if !ok || seen[name] {
			return
		}
		seen[name] = true
		hosts = append(hosts, g.Hosts...)
		for _, child := range g.Children {
			walk(child)
		}
	}
	walk(group)
	slices.Sort(hosts)
	return slices.Compact(hosts)
}

// HostGroups returns the groups a host belongs to directly or through group
// nesting (including "all"), sorted.
func (inv *Inventory) HostGroups(host string) []string {
	parents := map[string][]string{}
	for _, g := range inv.Groups {
		for _, child := range g.Children {
			parents[child] = append(parents[child], g.Name)
		}
	}
	h, ok := inv.Hosts[host]
	if !ok {
		return nil
	}
	seen := map[string]bool{}
	queue := slices.Clone(h.Groups)
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if seen[name] {
			continue
		}
		seen[name] = true
		queue = append(queue, parents[name]...)
	}
	return sortedKeys(seen)
}

func (inv *Inventory) host(name string) *Host {
	h, ok := inv.Hosts[name]
	if !ok {
		h = &Host{Name: name}
		inv.Hosts[name] = h
	}
	return h
}

// groupList snapshots the groups so the map can be modified while iterating.
func (inv *Inventory) groupList() []*Group {
	list := make([]*Group, 0, len(inv.Groups))
	for _, g := range inv.Groups {
		list = append(list, g)
	}
	return list
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// decodeJSON decodes numbers as json.Number so that large integers coming
// from Ansible are not rounded through float64.
func decodeJSON(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	return dec.Decode(v)
}
