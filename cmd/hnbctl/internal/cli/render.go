package cli

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"os"
	"strings"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"
)

func runRender(ctx context.Context, env *Env, args []string) Result {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var configPath, lockPath, chartPath, bindingsPath, outputPath, stage string
	fs.StringVar(&configPath, "f", "", "DeploymentConfig")
	fs.StringVar(&lockPath, "lock", "", "ReleaseLock override")
	fs.StringVar(&chartPath, "chart-package", "", "verified chart archive")
	fs.StringVar(&bindingsPath, "bindings", "", "identity/TLS/Secret binding values")
	fs.StringVar(&outputPath, "output", "", "private rendered manifest")
	fs.StringVar(&stage, "stage", "full", "startup or full candidate composition")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || configPath == "" || chartPath == "" || bindingsPath == "" || outputPath == "" {
		return fail(ReasonInvalidConfig, "render requires -f, --chart-package, --bindings and --output")
	}
	cfg, err := readConfig(configPath)
	if err != nil {
		return fail(ReasonInvalidConfig, "cannot read DeploymentConfig")
	}
	if problems := cfg.Validate(toolkit.CurrentEvidence); len(problems) > 0 {
		return fail(ReasonInvalidConfig, strings.Join(problems, "; "))
	}
	lock, _, err := readValidatedLock(cfg, lockPath)
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	bindings, err := os.ReadFile(bindingsPath)
	if err != nil {
		return fail(ReasonInvalidConfig, "binding values unavailable")
	}
	renderCtx, cancel := context.WithTimeout(ctx, cfg.Spec.Timeouts.Readiness)
	defer cancel()
	manifest, err := toolkit.RenderLockedChart(renderCtx, cfg, lock, chartPath, bindings, stage)
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	if err := toolkit.WriteRenderedManifest(outputPath, manifest); err != nil {
		return fail(ReasonOutputFailed, err.Error())
	}
	if err := json.NewEncoder(env.Stdout).Encode(map[string]any{
		"status": "rendered", "stage": stage, "namespace": cfg.Spec.Platform.Namespace,
		"output": outputPath, "installationExecuted": false,
	}); err != nil {
		return fail(ReasonOutputFailed, "cannot write rendering result")
	}
	return success("")
}
