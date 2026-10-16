package cli

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"path/filepath"
	"strings"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"
)

type prepareCLIObserver struct {
	encoder *json.Encoder
	err     error
}

func (o *prepareCLIObserver) Observe(p toolkit.PrepareProgress) {
	if o.err == nil {
		o.err = o.encoder.Encode(p)
	}
}

func runPrepare(ctx context.Context, env *Env, args []string) Result {
	fs := flag.NewFlagSet("prepare", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var configPath, lockPath, cacheDir, sourceDir, mirror string
	var preload bool
	fs.StringVar(&configPath, "f", "", "DeploymentConfig")
	fs.StringVar(&lockPath, "lock", "", "ReleaseLock override")
	fs.StringVar(&cacheDir, "cache-dir", "", "private verified cache directory")
	fs.StringVar(&sourceDir, "source-dir", "", "release package directory (default: lock directory)")
	fs.StringVar(&mirror, "mirror-origin", "", "explicit HTTPS registry origin; repository paths preserved")
	fs.BoolVar(&preload, "preload", false, "bootstrap preload (not yet authorized)")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || configPath == "" || cacheDir == "" {
		return fail(ReasonInvalidConfig, "prepare requires -f and --cache-dir; no positional arguments")
	}
	if preload {
		return fail(ReasonTargetActionMissing, "bootstrap preload is authorized only inside a bootstrap run (Day-0 ownership fence); prepare never contacts nodes and install execution remains gated")
	}
	cfg, err := readConfig(configPath)
	if err != nil {
		return fail(ReasonInvalidConfig, "cannot read DeploymentConfig")
	}
	if problems := cfg.ValidatePreparation(toolkit.CurrentEvidence, mirror); len(problems) > 0 {
		return fail(ReasonInvalidConfig, strings.Join(problems, "; "))
	}
	if cfg.Spec.Mode == toolkit.ModeManaged {
		return fail(ReasonTargetActionMissing, "managed preparation must use the formal Operation; no SSH fallback")
	}
	if cfg.Spec.Artifacts.Mode == toolkit.ArtifactsOffline {
		return fail(ReasonTargetActionMissing, "use bundle prepare/verify/import for local offline material; installation certification requires 7.2/7.3; no public requests performed")
	}
	lock, _, err := readValidatedLock(cfg, lockPath)
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	if sourceDir == "" {
		if lockPath == "" {
			lockPath = cfg.Spec.ReleaseLock
		}
		sourceDir = filepath.Dir(lockPath)
	}
	observer := &prepareCLIObserver{encoder: json.NewEncoder(env.Stderr)}
	d := toolkit.Downloader{MirrorBase: mirror, SourceDir: sourceDir, Observer: observer}
	prepareCtx, cancel := context.WithTimeout(ctx, cfg.Spec.Timeouts.Download)
	defer cancel()
	result, err := d.PrepareLock(prepareCtx, lock, cacheDir)
	if observer.err != nil {
		return fail(ReasonOutputFailed, "cannot write preparation progress")
	}
	if outputErr := json.NewEncoder(env.Stdout).Encode(result); outputErr != nil {
		return fail(ReasonOutputFailed, "cannot write preparation result")
	}
	if err != nil {
		return fail(ExitReason("PREPARE_FAILED"), toolkit.SafeReportText(err.Error()))
	}
	return success("")
}
