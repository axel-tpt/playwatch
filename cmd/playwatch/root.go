package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/axel-tpt/playwatch/internal/ansible"
	"github.com/axel-tpt/playwatch/internal/resource"
	"github.com/axel-tpt/playwatch/internal/store"
	"github.com/axel-tpt/playwatch/internal/ui"
)

// version is set at build time with -ldflags "-X main.version=…".
var version = "dev"

type options struct {
	inventories []string
}

func (o *options) client() *ansible.Client {
	return ansible.NewClient(o.inventories...)
}

func newRootCmd() *cobra.Command {
	opts := &options{}
	root := &cobra.Command{
		Use:           "playwatch",
		Short:         "A k9s-style terminal UI for Ansible",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runUI(cmd.Context(), opts)
		},
	}
	root.PersistentFlags().StringSliceVarP(&opts.inventories, "inventory", "i", nil,
		"inventory source (repeatable); defaults to Ansible's configuration")

	root.AddCommand(newHostsCmd(opts), newGroupsCmd(opts), newHostCmd(opts), newRunCmd(opts), newRunsCmd())
	return root
}

func newHostsCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "hosts",
		Short: "List inventory hosts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			inv, err := opts.client().Inventory(cmd.Context())
			if err != nil {
				return err
			}
			printWarnings(cmd.ErrOrStderr(), inv.Warnings)
			w := newTable(cmd.OutOrStdout())
			fmt.Fprintln(w, "NAME\tGROUPS\tVARS")
			for _, name := range inv.HostNames() {
				h := inv.Hosts[name]
				fmt.Fprintf(w, "%s\t%s\t%d\n", name, strings.Join(h.Groups, ","), len(h.Vars))
			}
			return w.Flush()
		},
	}
}

func newGroupsCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "groups",
		Short: "List inventory groups",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			inv, err := opts.client().Inventory(cmd.Context())
			if err != nil {
				return err
			}
			printWarnings(cmd.ErrOrStderr(), inv.Warnings)
			w := newTable(cmd.OutOrStdout())
			fmt.Fprintln(w, "NAME\tHOSTS\tCHILDREN")
			for _, name := range inv.GroupNames() {
				g := inv.Groups[name]
				fmt.Fprintf(w, "%s\t%d\t%s\n", name, len(inv.GroupHosts(name)), strings.Join(g.Children, ","))
			}
			return w.Flush()
		},
	}
}

func newHostCmd(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "host <name>",
		Short: "Print a host's variables as JSON",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			vars, err := opts.client().HostVars(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(vars)
		},
	}
}

// runUI starts the interactive TUI on the current directory's project.
func runUI(ctx context.Context, opts *options) error {
	runs := ui.NewRunList()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	inventory := strings.Join(opts.inventories, ",")
	if inventory == "" {
		inventory = "(ansible config)"
	}
	header := [][2]string{{"inventory", inventory}, {"project", dir}, {"version", version}}

	st, err := openStore()
	if err != nil {
		header = append(header, [2]string{"history", "disabled (" + err.Error() + ")"})
	}
	env := resource.Env{Ctx: ctx, Client: opts.client(), Dir: dir, Runs: runs, Store: st}
	return ui.Run(ctx, ui.Config{
		Context:   header,
		Resources: resource.All(env),
		Start:     "hosts",
		Runs:      runs,
	})
}

// keepRuns is how many finished runs the history keeps (all projects).
const keepRuns = 200

// openStore opens the run history and drops its oldest runs.
func openStore() (*store.Store, error) {
	dir, err := store.DefaultDir()
	if err != nil {
		return nil, err
	}
	st, err := store.Open(dir)
	if err != nil {
		return nil, err
	}
	if err := st.Prune(keepRuns); err != nil {
		return st, fmt.Errorf("prune run history: %w", err)
	}
	return st, nil
}

func newTable(w io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
}

func printWarnings(w io.Writer, warnings string) {
	if warnings = strings.TrimSpace(warnings); warnings != "" {
		fmt.Fprintln(w, warnings)
	}
}
