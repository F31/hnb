package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"
)

// runCollect exports a redacted support bundle. Every entry is constructed from
// typed, already-sanitized facts; free-form text is redacted before storage.
func runCollect(_ context.Context, env *Env, args []string) Result {
	fs := flag.NewFlagSet("collect", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var runDir, output, reasonCodes, remediationRef string
	fs.StringVar(&runDir, "run-dir", "", "private install run directory")
	fs.StringVar(&output, "output", "", "output archive path (exclusive create, 0600)")
	fs.StringVar(&reasonCodes, "reason-codes", "", "comma-separated reason codes to include")
	fs.StringVar(&remediationRef, "remediation-ref", "", "runbook reference for the failure")
	if err := fs.Parse(args); err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	if fs.NArg() != 0 {
		return fail(ReasonInvalidConfig, "unexpected collect arguments")
	}
	if output == "" {
		return fail(ReasonInvalidConfig, "--output is required")
	}
	if strings.TrimSpace(output) == "" {
		return fail(ReasonInvalidConfig, "--output must name a file")
	}

	bundleID, err := newBundleID()
	if err != nil {
		return fail(ReasonOutputFailed, "cannot generate bundle identity")
	}
	collector := &toolkit.DiagnosticCollector{BundleID: bundleID, CollectedAt: time.Now().UTC()}

	// Install run facts (phases, digests) when a run directory is supplied.
	if runDir != "" {
		run, err := toolkit.LoadRunState(runDir)
		if err != nil {
			return fail(ReasonInvalidConfig, err.Error())
		}
		collector.RunID = run.RunID
		phases := make([]map[string]any, 0, len(run.Phases))
		for name, phase := range run.Phases {
			if phase == nil {
				continue
			}
			phases = append(phases, map[string]any{
				"phase": name, "state": phase.State, "reasonCode": phase.ReasonCode,
				"attempts": phase.Attempts,
			})
		}
		if err := collector.AddJSON("install/run-summary.json", map[string]any{
			"runId": run.RunID, "configDigest": run.ConfigDigest, "lockDigest": run.LockDigest,
			"routingMode": run.RoutingMode, "ownership": run.Ownership,
			"status": run.Status, "startedAt": run.StartedAt, "phases": phases,
		}); err != nil {
			return fail(ReasonOutputFailed, err.Error())
		}
	} else {
		if err := collector.AddText("install/run-summary.txt", "no install run directory supplied"); err != nil {
			return fail(ReasonOutputFailed, err.Error())
		}
	}

	// Capability matrix and tool identity support failure localization.
	if err := collector.AddJSON("tool/identity.json", map[string]any{
		"toolVersion": VersionString,
		"collectedAt": time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		return fail(ReasonOutputFailed, err.Error())
	}

	codes := []string{}
	for _, code := range strings.Split(reasonCodes, ",") {
		if trimmed := strings.TrimSpace(code); trimmed != "" {
			codes = append(codes, trimmed)
		}
	}
	if err := collector.AddJSON("diagnosis/reason-codes.json", map[string]any{
		"reasonCodes": codes, "remediationRef": remediationRef,
	}); err != nil {
		return fail(ReasonOutputFailed, err.Error())
	}

	if err := collector.WriteArchiveFile(output); err != nil {
		return fail(ReasonOutputFailed, err.Error())
	}
	identity := collector.Identity()
	fmt.Fprintf(env.Stdout, "bundle=%s contentDigest=%s collectedAt=%s redacted=%v entries=%d\n",
		identity.BundleID, identity.ContentDigest, identity.CollectedAt.Format(time.RFC3339),
		identity.Redacted, len(identity.EntryPaths))
	return success("")
}

func newBundleID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
