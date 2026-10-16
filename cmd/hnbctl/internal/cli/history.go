package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"
)

func defaultRunsDir() (string, error) {
	root := os.Getenv("XDG_STATE_HOME")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot resolve state directory; provide --runs-dir")
		}
		root = filepath.Join(home, ".local", "state")
	}
	if !filepath.IsAbs(root) {
		return "", fmt.Errorf("XDG_STATE_HOME must be absolute; or provide --runs-dir")
	}
	return filepath.Join(root, "hnbctl", "runs"), nil
}

func resolveRunDir(dir, root, id string) (string, error) {
	if dir != "" {
		return dir, nil
	}
	if root == "" {
		var err error
		root, err = defaultRunsDir()
		if err != nil {
			return "", err
		}
	}
	runs, err := toolkit.ListRunHistory(root)
	if err != nil {
		return "", err
	}
	for _, run := range runs {
		if id == "" || id == run.RunID {
			return run.Directory, nil
		}
	}
	return "", fmt.Errorf("no matching install run found; run install first or provide --run-dir")
}

func runHistory(_ context.Context, env *Env, args []string) Result {
	fs := flag.NewFlagSet("history", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var root, format, state string
	var limit int
	fs.StringVar(&root, "runs-dir", "", "history root (default: XDG state home/hnbctl/runs)")
	fs.StringVar(&format, "format", "human", "human or json")
	fs.StringVar(&state, "state", "", "only runs in this state (e.g. running, blocked, succeeded, failed)")
	fs.IntVar(&limit, "limit", 20, "maximum records, newest first")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || limit < 1 || limit > 10000 {
		return fail(ReasonInvalidConfig, "history requires valid flags and --limit between 1 and 10000")
	}
	if format != "human" && format != "json" {
		return fail(ReasonInvalidConfig, "--format must be human or json")
	}
	if root == "" {
		var err error
		root, err = defaultRunsDir()
		if err != nil {
			return fail(ReasonInvalidConfig, err.Error())
		}
	}
	runs, err := toolkit.ListRunHistory(root)
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	if state != "" {
		filtered := runs[:0]
		for _, run := range runs {
			if run.Status == state {
				filtered = append(filtered, run)
			}
		}
		runs = filtered
	}
	total := len(runs)
	if len(runs) > limit {
		runs = runs[:limit]
	}
	if format == "json" {
		doc := struct {
			SchemaVersion string               `json:"schemaVersion"`
			Total         int                  `json:"total"`
			Runs          []toolkit.RunSummary `json:"runs"`
		}{"install-history/v1", total, runs}
		if err := json.NewEncoder(env.Stdout).Encode(doc); err != nil {
			return fail(ReasonOutputFailed, "cannot write installation history")
		}
	} else {
		fmt.Fprintln(env.Stdout, "RUN\tSTATE\tROUTE\tSTARTED\tFINISHED")
		for _, run := range runs {
			finished := "-"
			if !run.FinishedAt.IsZero() {
				finished = run.FinishedAt.Format(time.RFC3339)
			}
			fmt.Fprintf(env.Stdout, "%q\t%s\t%q\t%s\t%s\n", run.RunID, run.Status, run.RoutingMode, run.StartedAt.Format(time.RFC3339), finished)
		}
	}
	return success("")
}

func runLogs(ctx context.Context, env *Env, args []string) Result {
	fs := flag.NewFlagSet("logs", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var dir, root, id, format, phase, reason string
	var tail int
	var after int64
	var follow bool
	var interval time.Duration
	fs.StringVar(&dir, "run-dir", "", "private run directory")
	fs.StringVar(&root, "runs-dir", "", "history root")
	fs.StringVar(&id, "run-id", "", "run id (default: latest)")
	fs.StringVar(&format, "format", "human", "human or json; follow emits JSON Lines")
	fs.IntVar(&tail, "tail", 100, "initial event count; 0 returns all events")
	fs.Int64Var(&after, "after", 0, "only events with sequence greater than this value")
	fs.StringVar(&phase, "phase", "", "only events for this phase name")
	fs.StringVar(&reason, "reason", "", "only events whose reason code equals this value")
	fs.BoolVar(&follow, "follow", false, "follow state transitions until terminal or canceled")
	fs.DurationVar(&interval, "interval", time.Second, "follow polling interval")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || tail < 0 || after < 0 || interval < 100*time.Millisecond {
		return fail(ReasonInvalidConfig, "invalid logs flags; --tail and --after must be nonnegative and --interval at least 100ms")
	}
	if format != "human" && format != "json" {
		return fail(ReasonInvalidConfig, "--format must be human or json")
	}
	keep := func(event toolkit.RunEvent) bool {
		if after > 0 && event.Sequence <= after {
			return false
		}
		if phase != "" && string(event.Phase) != phase {
			return false
		}
		if reason != "" && event.ReasonCode != reason {
			return false
		}
		return true
	}
	dir, err := resolveRunDir(dir, root, id)
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	var last int64
	first := true
	for {
		run, err := toolkit.LoadRunState(dir)
		if err != nil {
			return fail(ReasonInvalidConfig, err.Error())
		}
		if id != "" && id != run.RunID {
			return fail(ReasonInvalidConfig, "stored run id does not match --run-id")
		}
		events := make([]toolkit.RunEvent, 0, len(run.Events))
		for _, event := range run.Events {
			if keep(event) {
				events = append(events, event)
			}
		}
		if first && tail > 0 && len(events) > tail {
			events = events[len(events)-tail:]
		}
		if format == "json" && !follow {
			doc := struct {
				SchemaVersion string             `json:"schemaVersion"`
				RunID         string             `json:"runId"`
				Events        []toolkit.RunEvent `json:"events"`
			}{"install-logs/v1", toolkit.SafeReportText(run.RunID), events}
			if err := json.NewEncoder(env.Stdout).Encode(doc); err != nil {
				return fail(ReasonOutputFailed, "cannot write installation logs")
			}
			return success("")
		}
		for _, event := range events {
			if event.Sequence <= last {
				continue
			}
			if format == "json" {
				if err := json.NewEncoder(env.Stdout).Encode(event); err != nil {
					return fail(ReasonOutputFailed, "cannot write installation event")
				}
			} else {
				fmt.Fprintf(env.Stdout, "%s #%d %s phase=%q state=%q reason=%q\n", event.Time.Format(time.RFC3339), event.Sequence, event.Kind, event.Phase, event.State, event.ReasonCode)
			}
			last = event.Sequence
		}
		first = false
		if !follow || run.Status != toolkit.PhaseRunningState {
			return success("")
		}
		if err := waitForRunPoll(ctx, interval); err != nil {
			return fail(ReasonPreflightBlocked, "log following canceled; installation was not changed")
		}
	}
}

func waitForRunPoll(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
