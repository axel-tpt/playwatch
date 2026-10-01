package ansible

import (
	"slices"
	"testing"
)

func TestRunOptionsArgs(t *testing.T) {
	o := RunOptions{
		Limit:             "web",
		Tags:              "deploy,config",
		SkipTags:          " slow ",
		Check:             true,
		Diff:              true,
		ExtraVars:         []string{"version=1.2 env=prod", "", "@vars.yml"},
		VaultPasswordFile: "~/.vault_pass",
		VaultIDs:          []string{"prod@prompt"},
		AskVaultPass:      true,
		Extra:             []string{"-vv"},
	}
	want := []string{
		"--limit", "web", "--tags", "deploy,config", "--skip-tags", "slow", "--check", "--diff",
		"--extra-vars", "version=1.2 env=prod", "--extra-vars", "@vars.yml",
		"--vault-password-file", "~/.vault_pass", "--vault-id", "prod@prompt", "-vv",
	}
	if got := o.Args(); !slices.Equal(got, want) {
		t.Errorf("Args() =\n%q\nwant\n%q", got, want)
	}
	if got := (RunOptions{}).Args(); len(got) != 0 {
		t.Errorf("empty options give %q", got)
	}
}

func TestRunOptionsWithLimit(t *testing.T) {
	tests := []struct {
		extra, want []string
	}{
		{nil, nil},
		{[]string{"--limit", "web", "--diff"}, []string{"--diff"}},
		{[]string{"-l", "web", "-t", "x"}, []string{"-t", "x"}},
		{[]string{"--limit=web"}, []string{}},
		{[]string{"-lweb", "-vv"}, []string{"-vv"}},
	}
	for _, tt := range tests {
		o := RunOptions{Limit: "old", Check: true, Extra: tt.extra}.WithLimit("web2")
		if o.Limit != "web2" || !o.Check || !slices.Equal(o.Extra, tt.want) {
			t.Errorf("WithLimit with extra %q = %+v, want limit web2 and extra %q", tt.extra, o, tt.want)
		}
	}
}

func TestFailedHosts(t *testing.T) {
	stats := map[string]HostStats{
		"web1": {OK: 3},
		"web2": {OK: 1, Failures: 1},
		"db1":  {Unreachable: 1},
	}
	got := FailedHosts(stats, []string{"web1", "web2", "db1", "gone"})
	if want := []string{"web2", "db1"}; !slices.Equal(got, want) {
		t.Errorf("FailedHosts = %v, want %v", got, want)
	}
}
