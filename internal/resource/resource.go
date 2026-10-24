// Package resource declares the resources playwatch can list (hosts, groups,
// playbooks, runs). Each one is a ui.Resource built on top of
// internal/ansible; adding a resource only means adding a file here and
// listing it in All.
package resource

import (
	"context"

	"github.com/axel-tpt/playwatch/internal/ansible"
	"github.com/axel-tpt/playwatch/internal/store"
	"github.com/axel-tpt/playwatch/internal/ui"
)

// Env is what resources need to load data and start runs.
type Env struct {
	// Ctx is the application context, cancelled on quit.
	Ctx    context.Context
	Client *ansible.Client
	// Dir is the absolute Ansible project directory (where playbooks are
	// looked up, and the scope of the run history).
	Dir string
	// PlaybookDir is where playbooks are listed from (relative to Dir unless
	// absolute); empty means DefaultPlaybookDir.
	PlaybookDir string
	Runs        *ui.RunList
	// Store persists runs; nil disables persistence.
	Store *store.Store
}

// RunEnv returns what run views need.
func (env Env) RunEnv() ui.RunEnv {
	return ui.RunEnv{Client: env.Client, Runs: env.Runs, Store: env.Store, Project: env.Dir}
}

// All returns every resource, in the order shown in the help.
func All(env Env) []ui.Resource {
	return []ui.Resource{
		Hosts(env, ""),
		Groups(env),
		Playbooks(env),
		Roles(env),
		Runs(env),
	}
}
