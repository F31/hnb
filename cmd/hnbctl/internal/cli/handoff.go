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

// runHandoff establishes the first management relationship for an install run.
// It submits a formal registration intent and reads server-authoritative agent
// observation; the local run/receipt is attached as correlation evidence only.
func runHandoff(ctx context.Context, env *Env, args []string) Result {
	fs := flag.NewFlagSet("handoff", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var opts runFlags
	var targetID, idemKey, corrID, format string
	fs.StringVar(&opts.dir, "run-dir", "", "private install run directory")
	fs.StringVar(&targetID, "target-id", "", "runtime target id being handed off")
	fs.StringVar(&idemKey, "idempotency-key", "", "stable idempotency key for the registration intent")
	fs.StringVar(&corrID, "correlation-id", "", "request correlation id")
	fs.StringVar(&format, "format", "human", "stdout format: human or json")
	var baseURL, tokenFile, tenant string
	var timeout time.Duration
	fs.StringVar(&baseURL, "platform-url", "", "platform API base URL (HTTPS)")
	fs.StringVar(&tokenFile, "token-file", "", "file holding the API bearer token")
	fs.StringVar(&tenant, "tenant-scope", "", "authenticated tenant scope")
	fs.DurationVar(&timeout, "timeout", 30*time.Second, "platform request timeout")
	if err := fs.Parse(args); err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	if fs.NArg() != 0 {
		return fail(ReasonInvalidConfig, "unexpected handoff arguments")
	}
	if opts.dir == "" || targetID == "" || idemKey == "" {
		return fail(ReasonInvalidConfig, "handoff requires --run-dir, --target-id and --idempotency-key")
	}
	if format != "human" && format != "json" {
		return fail(ReasonInvalidConfig, "--format must be human or json")
	}
	run, err := toolkit.LoadRunState(opts.dir)
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	client, err := newPlatformClient(clusterFlags{baseURL: baseURL, tokenFile: tokenFile, tenant: tenant, timeout: timeout})
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result, err := client.Handoff(requestCtx, targetID, idemKey, corrID, run, time.Now().UTC())
	if format == "json" {
		if encErr := json.NewEncoder(env.Stdout).Encode(result); encErr != nil {
			return fail(ReasonOutputFailed, "cannot write handoff result")
		}
	} else {
		fmt.Fprintf(env.Stdout, "target=%s complete=%v operation=%s\n", result.TargetID, result.Complete, result.RegistrationOperationID)
		for _, reason := range result.Reasons {
			fmt.Fprintf(env.Stdout, "  incomplete: %s\n", reason)
		}
		if result.Observation != nil {
			fmt.Fprintf(env.Stdout, "  agent authenticated=%v observedAt=%s\n", result.Observation.Authenticated, result.Observation.ObservedAt.Format(time.RFC3339))
		}
	}
	if err != nil {
		return platformResult(err)
	}
	if !result.Complete {
		// A receipt alone never completes handoff; the managed layer stays unverified.
		return fail(ReasonPlatformNotVerified, "handoff incomplete: registration Operation and live agent observation are both required")
	}
	return success("")
}
