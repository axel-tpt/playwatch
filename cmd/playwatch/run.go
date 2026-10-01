package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/axel-tpt/playwatch/internal/ansible"
	"github.com/axel-tpt/playwatch/internal/store"
)

// exitCodeError makes playwatch exit with the given code without printing anything
// more (the reason has already been printed).
type exitCodeError struct{ code int }

func (e exitCodeError) Error() string { return fmt.Sprintf("exit status %d", e.code) }

func newRunCmd(opts *options) *cobra.Command {
	var (
		ropts   ansible.RunOptions
		jsonOut bool
		raw     bool
	)
	cmd := &cobra.Command{
		Use:   "run <playbook>",
		Short: "Run a playbook and stream its events",
		Long: "Run a playbook and print one line per task result. The exit code is\n" +
			"ansible-playbook's (2 when a host failed, 4 when one was unreachable).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client := opts.client()
			popts := ansible.PlaybookOptions{Playbook: args[0], Args: ropts.Args()}
			var outputs []io.Writer
			if raw {
				outputs = append(outputs, cmd.OutOrStdout())
			}
			rec := startRecording(cmd.ErrOrStderr(), client, popts.Playbook, ropts)
			if rec != nil {
				outputs = append(outputs, rec.Output())
			}
			popts.Output = io.MultiWriter(outputs...)

			run, err := client.RunPlaybook(cmd.Context(), popts)
			if err != nil {
				if rec != nil {
					_ = rec.Finish(store.StateFailed, -1, nil)
				}
				return err
			}

			out := cmd.OutOrStdout()
			var stats map[string]ansible.HostStats
			for e := range run.Events {
				if rec != nil {
					if err := rec.Record(e); err != nil {
						fmt.Fprintln(cmd.ErrOrStderr(), "playwatch: run not fully saved:", err)
						rec = nil
					}
				}
				if e.Type == ansible.EventStats {
					stats = e.Stats
				}
				switch {
				case jsonOut:
					printEventJSON(out, e)
				case !raw:
					printEvent(out, e)
				}
			}
			result := runResult(cmd.ErrOrStderr(), run, raw)
			if rec != nil {
				if err := rec.Finish(cliRunState(cmd.Context().Err(), run.Wait()), run.ExitCode(), stats); err != nil {
					fmt.Fprintln(cmd.ErrOrStderr(), "playwatch: run not fully saved:", err)
				}
			}
			return result
		},
	}
	f := cmd.Flags()
	f.StringVarP(&ropts.Limit, "limit", "l", "", "limit the run to a host pattern")
	f.StringVarP(&ropts.Tags, "tags", "t", "", "only run tasks tagged with these tags")
	f.StringVar(&ropts.SkipTags, "skip-tags", "", "skip tasks tagged with these tags")
	f.BoolVarP(&ropts.Check, "check", "C", false, "report changes without making them")
	f.BoolVarP(&ropts.Diff, "diff", "D", false, "show what changes in files and templates")
	f.StringArrayVarP(&ropts.ExtraVars, "extra-vars", "e", nil, "extra variables: key=value, JSON or @file (repeatable)")
	f.StringVar(&ropts.VaultPasswordFile, "vault-password-file", "", "vault password file")
	f.StringArrayVar(&ropts.VaultIDs, "vault-id", nil, "vault identity, label@source (repeatable)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "print raw events as JSON lines")
	cmd.Flags().BoolVar(&raw, "raw", false, "print ansible-playbook's own output instead of events")
	cmd.MarkFlagsMutuallyExclusive("json", "raw")
	return cmd
}

// startRecording saves the run in the history. Failing to do so only warns:
// the run itself must go on.
func startRecording(stderr io.Writer, client *ansible.Client, playbook string, ropts ansible.RunOptions) *store.Recorder {
	st, err := openStore()
	if err == nil {
		var project string
		if project, err = os.Getwd(); err == nil {
			var rec *store.Recorder
			rec, err = st.Create(store.Meta{
				Playbook:    playbook,
				Project:     project,
				Inventories: client.Inventories,
				Options:     &ropts,
				Args:        ropts.Args(),
			})
			if err == nil {
				return rec
			}
		}
	}
	fmt.Fprintln(stderr, "playwatch: run will not be saved:", err)
	return nil
}

func cliRunState(ctxErr, err error) string {
	switch {
	case ctxErr != nil:
		return store.StateInterrupted
	case err != nil:
		return store.StateFailed
	}
	return store.StateOK
}

// runResult turns the end of a run into playwatch's exit code. When the events were
// shown instead of Ansible's output, Ansible's stderr is printed on failure.
func runResult(stderr io.Writer, run *ansible.Run, raw bool) error {
	err := run.Wait()
	if err == nil {
		return nil
	}
	var cmdErr *ansible.CommandError
	if !errors.As(err, &cmdErr) || run.ExitCode() <= 0 {
		return err
	}
	if !raw {
		printWarnings(stderr, cmdErr.Stderr)
	}
	return exitCodeError{code: run.ExitCode()}
}

func printEventJSON(w io.Writer, e ansible.Event) {
	data, err := json.Marshal(e)
	if err != nil {
		fmt.Fprintf(os.Stderr, "encode event: %v\n", err)
		return
	}
	fmt.Fprintf(w, "%s\n", data)
}

func printEvent(w io.Writer, e ansible.Event) {
	switch e.Type {
	case ansible.EventPlayStart:
		fmt.Fprintf(w, "\nPLAY %s\n", e.Play)
	case ansible.EventTaskStart:
		kind := "TASK"
		if e.IsHandler {
			kind = "HANDLER"
		}
		fmt.Fprintf(w, "%s %s\n", kind, e.Task)
	case ansible.EventStats:
		fmt.Fprintln(w, "\nRECAP")
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, host := range sortedHosts(e.Stats) {
			s := e.Stats[host]
			fmt.Fprintf(tw, "  %s\tok=%d\tchanged=%d\tfailed=%d\tunreachable=%d\tskipped=%d\trescued=%d\tignored=%d\n",
				host, s.OK, s.Changed, s.Failures, s.Unreachable, s.Skipped, s.Rescued, s.Ignored)
		}
		_ = tw.Flush()
	default:
		status := e.Status()
		if status == ansible.StatusNone || status == ansible.StatusRunning {
			return
		}
		line := fmt.Sprintf("  %-11s %s", status, e.Host)
		if msg := resultMsg(e); msg != "" && status != ansible.StatusOK && status != ansible.StatusChanged {
			line += "  " + msg
		}
		fmt.Fprintln(w, line)
	}
}

// resultMsg extracts the module's "msg" from a result, on one line.
func resultMsg(e ansible.Event) string {
	var res struct {
		Msg any `json:"msg"`
	}
	if json.Unmarshal(e.Result, &res) != nil || res.Msg == nil {
		return ""
	}
	msg, ok := res.Msg.(string)
	if !ok {
		data, _ := json.Marshal(res.Msg)
		msg = string(data)
	}
	return strings.Join(strings.Fields(msg), " ")
}

func sortedHosts(stats map[string]ansible.HostStats) []string {
	hosts := make([]string, 0, len(stats))
	for h := range stats {
		hosts = append(hosts, h)
	}
	slices.Sort(hosts)
	return hosts
}
