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

// runVerify renders the required acceptance check set. Probes are supplied by
// the caller; with no probe source wired the checks report themselves as
// unverified rather than assuming success.
func runVerify(ctx context.Context, env *Env, args []string) Result {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var level, format, runDir string
	fs.StringVar(&level, "level", "", "delivery layer: node|cluster|platform|managed")
	fs.StringVar(&format, "format", "human", "stdout format: human or json")
	fs.StringVar(&runDir, "run-dir", "", "optional install run directory for correlation")
	if err := fs.Parse(args); err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	if fs.NArg() != 0 {
		return fail(ReasonInvalidConfig, "unexpected verify arguments")
	}
	if level != "" {
		valid := false
		for _, layer := range toolkit.DeliveryLayers {
			if string(layer) == level {
				valid = true
			}
		}
		if !valid {
			return fail(ReasonInvalidConfig, "--level must be node, cluster, platform or managed")
		}
	}
	if format != "human" && format != "json" {
		return fail(ReasonInvalidConfig, "--format must be human or json")
	}

	// Without a bound deployment the installer has no authenticated observation
	// source for any layer. Emitting the catalog keeps the required checks
	// visible, but every one of them must stay unverified.
	specs := toolkit.PendingVerificationSpecs()
	if level != "" {
		selected := make([]toolkit.VerifySpec, 0)
		for _, spec := range specs {
			if string(spec.Layer) == level {
				selected = append(selected, spec)
			}
		}
		specs = selected
	}
	report := toolkit.RunVerify(ctx, specs, time.Now().UTC())
	if level != "" {
		selected := make([]toolkit.VerifyLayer, 0, 1)
		for _, layer := range report.Layers {
			if string(layer.Layer) == level {
				selected = append(selected, layer)
			}
		}
		// The delivery verdict still accounts for all required layers. A scoped
		// display must never promote a node check into full platform acceptance.
		report.Layers = selected
	}
	unverified := 0
	for _, layer := range report.Layers {
		for _, check := range layer.Checks {
			if check.State != toolkit.LayerPassed {
				unverified++
			}
		}
	}
	if format == "json" {
		if err := json.NewEncoder(env.Stdout).Encode(report); err != nil {
			return fail(ReasonOutputFailed, "cannot write verify report")
		}
	} else {
		toolkit.FormatVerifyHuman(env.Stdout, report)
	}
	return fail(ReasonPlatformNotVerified, fmt.Sprintf("verify incomplete: %d required checks unverified; run against a bound deployment", unverified))
}
