package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/axel-tpt/playwatch/internal/ansible"
)

func newRunsCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "runs",
		Short: "List the run history of the current project",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			st, err := openStore()
			if err != nil {
				return err
			}
			project := ""
			if !all {
				if project, err = os.Getwd(); err != nil {
					return fmt.Errorf("get working directory: %w", err)
				}
			}
			metas, err := st.List(project)
			if err != nil {
				return err
			}
			w := newTable(cmd.OutOrStdout())
			fmt.Fprintln(w, "ID\tPLAYBOOK\tSTATE\tRC\tDURATION\tFAILED HOSTS\tARGS")
			for _, m := range metas {
				failed := ansible.FailedHosts(m.Stats, sortedHosts(m.Stats))
				fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\t%s\t%s\n", m.ID, m.Playbook, m.State, m.ExitCode,
					m.Duration().Round(100*time.Millisecond), strings.Join(failed, ","), strings.Join(m.Args, " "))
			}
			return w.Flush()
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "list the runs of every project")
	return cmd
}
