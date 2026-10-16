package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"
)

type runFlags struct {
	dir      string
	runID    string
	format   string
	watch    bool
	interval time.Duration
}

// parseRunFlags parses the shared run flags. resume keeps an explicit --run-dir;
// status additionally resolves the recorded run from the install history so the
// operator can watch the latest run without remembering its directory.
func parseRunFlags(name string, args []string) (runFlags, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var out runFlags
	var runsDir string
	fs.StringVar(&out.dir, "run-dir", "", "private install run directory")
	fs.StringVar(&runsDir, "runs-dir", "", "install history root (default: XDG state home/hnbctl/runs)")
	fs.StringVar(&out.runID, "run-id", "", "expected run id (must match the stored run)")
	fs.StringVar(&out.format, "format", "human", "stdout format: human or json")
	if name == "status" {
		fs.BoolVar(&out.watch, "watch", false, "refresh progress until the run reaches a terminal state")
		fs.DurationVar(&out.interval, "interval", time.Second, "watch polling interval")
	}
	if err := fs.Parse(args); err != nil {
		return out, err
	}
	if fs.NArg() != 0 {
		return out, fmt.Errorf("%s does not accept positional arguments", name)
	}
	if name != "status" && out.dir == "" {
		return out, fmt.Errorf("%s requires --run-dir", name)
	}
	if out.dir == "" {
		dir, err := resolveRunDir("", runsDir, out.runID)
		if err != nil {
			return out, err
		}
		out.dir = dir
	}
	if out.format != "human" && out.format != "json" {
		return out, fmt.Errorf("--format must be human or json")
	}
	if name == "status" && out.interval < 100*time.Millisecond {
		return out, fmt.Errorf("--interval must be at least 100ms")
	}
	return out, nil
}

func runStatus(ctx context.Context, env *Env, args []string) Result {
	opts, err := parseRunFlags("status", args)
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	if opts.watch {
		return runStatusWatch(ctx, env, opts)
	}
	return runStatusOnce(env, opts)
}

func runStatusOnce(env *Env, opts runFlags) Result {
	run, err := toolkit.LoadRunState(opts.dir)
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	if opts.runID != "" && run.RunID != opts.runID {
		return fail(ReasonInvalidConfig, "stored run id does not match --run-id")
	}
	report := toolkit.NewDeliveryReport(run.RunID, toolkit.LayerOutcomesFromRun(run))
	if opts.format == "json" {
		if err := json.NewEncoder(env.Stdout).Encode(report); err != nil {
			return fail(ReasonOutputFailed, "cannot write status report")
		}
	} else {
		toolkit.FormatDeliveryHuman(env.Stdout, report)
	}
	return statusResult(report, run)
}

func runStatusWatch(ctx context.Context, env *Env, opts runFlags) Result {
	first := true
	for {
		run, err := toolkit.LoadRunState(opts.dir)
		if err != nil {
			return fail(ReasonInvalidConfig, err.Error())
		}
		if opts.runID != "" && run.RunID != opts.runID {
			return fail(ReasonInvalidConfig, "stored run id does not match --run-id")
		}
		if opts.format == "json" {
			doc := struct {
				SchemaVersion string                  `json:"schemaVersion"`
				Progress      toolkit.RunProgress     `json:"progress"`
				Report        *toolkit.DeliveryReport `json:"report"`
			}{"status-watch/v1", toolkit.ProgressFromRun(run, time.Now().UTC()), toolkit.NewDeliveryReport(run.RunID, toolkit.LayerOutcomesFromRun(run))}
			if err := json.NewEncoder(env.Stdout).Encode(doc); err != nil {
				return fail(ReasonOutputFailed, "cannot write status watch frame")
			}
		} else {
			if !first {
				fmt.Fprintln(env.Stdout, "----")
			}
			toolkit.FormatProgressHuman(env.Stdout, toolkit.ProgressFromRun(run, time.Now().UTC()))
		}
		first = false
		if run.Status != toolkit.PhaseRunningState {
			return statusResult(toolkit.NewDeliveryReport(run.RunID, toolkit.LayerOutcomesFromRun(run)), run)
		}
		timer := time.NewTimer(opts.interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fail(ReasonPreflightBlocked, "status watch canceled; the installation was not changed")
		case <-timer.C:
		}
	}
}

// statusResult maps run state plus the layered report to an exit code. A run is
// successful only when every layer is independently verified.
func statusResult(report *toolkit.DeliveryReport, run *toolkit.InstallRun) Result {
	if report.CompleteDelivery && run.Status == toolkit.PhaseSucceededState {
		return success("")
	}
	reason := ReasonPreflightBlocked
	switch report.Overall {
	case toolkit.DeliveryPartial:
		reason = ReasonPlatformNotVerified
	case toolkit.DeliveryFailed:
		reason = ReasonOutputFailed
	}
	return fail(reason, "run "+run.RunID+" is "+run.Status+"; delivery is "+report.Overall)
}

func runResume(_ context.Context, env *Env, args []string) Result {
	opts, err := parseRunFlags("resume", args)
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	run, err := toolkit.LoadRunState(opts.dir)
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	if opts.runID != "" && run.RunID != opts.runID {
		return fail(ReasonInvalidConfig, "stored run id does not match --run-id")
	}
	if run.Status == toolkit.PhaseSucceededState {
		return fail(ReasonInvalidConfig, "run already succeeded; resume is not applicable")
	}
	plan := toolkit.PlanResumeFromState(run)
	if opts.format == "json" {
		doc := map[string]any{"runId": run.RunID, "status": run.Status, "configDigest": run.ConfigDigest, "lockDigest": run.LockDigest, "resume": plan}
		if err := json.NewEncoder(env.Stdout).Encode(doc); err != nil {
			return fail(ReasonOutputFailed, "cannot write resume plan")
		}
	} else {
		toolkit.FormatResumeHuman(env.Stdout, run, plan)
	}
	return fail(ReasonPlatformNotVerified, "resume plan computed; execution requires the certified install route")
}
