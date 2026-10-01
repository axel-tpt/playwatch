package ansible

import "strings"

// RunOptions are the user-facing options of an ansible-playbook run. They are
// stored with each run so that it can be rerun or edited.
type RunOptions struct {
	Limit    string `json:"limit,omitempty"`
	Tags     string `json:"tags,omitempty"`
	SkipTags string `json:"skip_tags,omitempty"`
	Check    bool   `json:"check,omitempty"`
	Diff     bool   `json:"diff,omitempty"`
	// ExtraVars are passed as repeated -e values ("k=v k2=v2", JSON or
	// @file).
	ExtraVars         []string `json:"extra_vars,omitempty"`
	VaultPasswordFile string   `json:"vault_password_file,omitempty"`
	VaultIDs          []string `json:"vault_ids,omitempty"`
	// AskVaultPass records that a vault password was typed in for the run.
	// The password itself is never stored (see PlaybookOptions).
	AskVaultPass bool `json:"ask_vault_pass,omitempty"`
	// Extra holds raw ansible-playbook arguments.
	Extra []string `json:"extra,omitempty"`
}

// Args returns the ansible-playbook arguments for the options. The typed-in
// vault password is not part of them: the runner adds its own argument.
func (o RunOptions) Args() []string {
	var args []string
	add := func(flag, value string) {
		if value = strings.TrimSpace(value); value != "" {
			args = append(args, flag, value)
		}
	}
	add("--limit", o.Limit)
	add("--tags", o.Tags)
	add("--skip-tags", o.SkipTags)
	if o.Check {
		args = append(args, "--check")
	}
	if o.Diff {
		args = append(args, "--diff")
	}
	for _, ev := range o.ExtraVars {
		add("--extra-vars", ev)
	}
	add("--vault-password-file", o.VaultPasswordFile)
	for _, id := range o.VaultIDs {
		add("--vault-id", id)
	}
	return append(args, o.Extra...)
}

// WithLimit returns the options limited to limit, dropping any --limit from
// the raw arguments.
func (o RunOptions) WithLimit(limit string) RunOptions {
	o.Limit = limit
	o.Extra = withoutLimit(o.Extra)
	return o
}

// withoutLimit returns args without any --limit/-l option.
func withoutLimit(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--limit" || a == "-l":
			i++ // skip the value
		case strings.HasPrefix(a, "--limit="):
		case strings.HasPrefix(a, "-l") && len(a) > 2:
		default:
			out = append(out, a)
		}
	}
	return out
}

// FailedHosts returns the hosts of a recap that failed or were unreachable,
// in the order of hosts.
func FailedHosts(stats map[string]HostStats, hosts []string) []string {
	var failed []string
	for _, h := range hosts {
		if s, ok := stats[h]; ok && (s.Failures > 0 || s.Unreachable > 0) {
			failed = append(failed, h)
		}
	}
	return failed
}
