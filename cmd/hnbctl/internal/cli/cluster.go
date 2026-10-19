package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"
)

type clusterFlags struct {
	baseURL            string
	tokenFile          string
	tenant             string
	idemKey            string
	corrID             string
	targetID           string
	kind               string
	offering           string
	format             string
	operation          string
	displayName        string
	credentialProvider string
	credentialScope    string
	credentialName     string
	credentialVersion  string
	timeout            time.Duration
}

func parseClusterFlags(sub string, args []string) (clusterFlags, error) {
	fs := flag.NewFlagSet("cluster "+sub, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var c clusterFlags
	fs.StringVar(&c.baseURL, "platform-url", "", "platform API base URL (HTTPS)")
	fs.StringVar(&c.tokenFile, "token-file", "", "file holding the API bearer token (never passed on the command line)")
	fs.StringVar(&c.tenant, "tenant-scope", "", "authenticated tenant scope for secret references")
	fs.StringVar(&c.idemKey, "idempotency-key", "", "stable idempotency key for this intent")
	fs.StringVar(&c.corrID, "correlation-id", "", "request correlation id (UUID)")
	fs.StringVar(&c.targetID, "target-id", "", "existing runtime target id (import)")
	fs.StringVar(&c.kind, "target-kind", "", "target kind, e.g. kubernetes")
	fs.StringVar(&c.offering, "offering-id", "", "service offering id (create)")
	fs.StringVar(&c.format, "format", "human", "stdout format: human or json")
	fs.StringVar(&c.operation, "operation", "", "operation id (operation get)")
	fs.StringVar(&c.displayName, "display-name", "", "runtime target display name")
	fs.StringVar(&c.credentialProvider, "credential-provider", "", "registered kubeconfig secret provider")
	fs.StringVar(&c.credentialScope, "credential-scope", "", "registered kubeconfig secret scope")
	fs.StringVar(&c.credentialName, "credential-name", "", "registered kubeconfig secret name")
	fs.StringVar(&c.credentialVersion, "credential-version", "", "registered kubeconfig secret version")
	fs.DurationVar(&c.timeout, "timeout", 30*time.Second, "platform request timeout")
	if err := fs.Parse(args); err != nil {
		return c, err
	}
	if fs.NArg() != 0 {
		return c, fmt.Errorf("unexpected arguments")
	}
	return c, nil
}

// platformClientOverride, when non-nil, replaces the HTTP client used for
// platform requests. Test-only: lets a fixture TLS server (self-signed, no CA
// trust) be reached through the CLI. Production code never sets it.
var platformClientOverride *http.Client

// newPlatformClient builds the client from flags. The token is read from a
// private file so it never appears in argv or the process list.
func newPlatformClient(c clusterFlags) (*toolkit.PlatformClient, error) {
	if c.baseURL == "" {
		return nil, fmt.Errorf("--platform-url is required")
	}
	if !strings.HasPrefix(c.baseURL, "https://") {
		return nil, fmt.Errorf("--platform-url must be an HTTPS origin")
	}
	token := ""
	if c.tokenFile != "" {
		fi, err := os.Stat(c.tokenFile)
		if err != nil || !fi.Mode().IsRegular() {
			return nil, fmt.Errorf("token file is unavailable")
		}
		if fi.Mode().Perm()&0077 != 0 {
			return nil, fmt.Errorf("token file must be private (0600)")
		}
		data, err := os.ReadFile(c.tokenFile)
		if err != nil {
			return nil, fmt.Errorf("token file is unreadable")
		}
		token = strings.TrimSpace(string(data))
	}
	client := &http.Client{Timeout: c.timeout}
	if platformClientOverride != nil {
		client = platformClientOverride
	}
	return &toolkit.PlatformClient{BaseURL: c.baseURL, Token: token, TenantScope: c.tenant, Client: client}, nil
}

// platformResult maps a client outcome to an exit code. An unavailable platform
// reports a connection failure and never falls back to local execution.
func platformResult(err error) Result {
	switch {
	case err == nil:
		return success("")
	case errors.Is(err, toolkit.ErrPlatformUnavailable):
		return fail(ReasonAPIUnreachable, "platform API unavailable; no local fallback and no fabricated operation: "+err.Error())
	case errors.Is(err, toolkit.ErrNotAuthenticated):
		return fail(ReasonInvalidConfig, "missing or unusable platform credential")
	case errors.Is(err, toolkit.ErrRequestRejected):
		return fail(ReasonInvalidConfig, err.Error())
	default:
		return fail(ReasonInvalidConfig, err.Error())
	}
}

func runCluster(ctx context.Context, env *Env, args []string) Result {
	if len(args) == 0 {
		return fail(ReasonInvalidConfig, "cluster subcommand required: create|import|list")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "create", "import":
		return runClusterSubmit(ctx, env, sub, rest)
	case "list":
		return runClusterList(ctx, env, rest)
	default:
		return fail(ReasonInvalidConfig, "unknown cluster subcommand: "+sub)
	}
}

// runClusterList reads the server-authoritative runtime-target list. It never
// fabricates rows or falls back to a local view when the platform is down.
func runClusterList(ctx context.Context, env *Env, args []string) Result {
	c, err := parseClusterFlags("list", args)
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	client, err := newPlatformClient(c)
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	requestCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	targets, err := client.ListRuntimeTargets(requestCtx)
	if err != nil {
		return platformResult(err)
	}
	if c.format == "json" {
		doc := struct {
			SchemaVersion string                         `json:"schemaVersion"`
			Total         int                            `json:"total"`
			Targets       []toolkit.RuntimeTargetSummary `json:"targets"`
		}{"cluster-list/v1", len(targets), targets}
		if err := json.NewEncoder(env.Stdout).Encode(doc); err != nil {
			return fail(ReasonOutputFailed, "cannot write cluster list")
		}
	} else {
		if len(targets) == 0 {
			fmt.Fprintln(env.Stdout, "no clusters found")
			return success("")
		}
		fmt.Fprintln(env.Stdout, "TARGET\tNAME\tTYPE\tDIST\tSTATUS\tACTIVE\tCREATED")
		for _, t := range targets {
			fmt.Fprintf(env.Stdout, "%s\t%s\t%s\t%s\t%s\t%v\t%s\n", t.ID, t.Name, t.TargetType, t.Distribution, t.Status, t.IsActive, t.CreatedAt.Format(time.RFC3339))
		}
	}
	return success("")
}

func runClusterSubmit(ctx context.Context, env *Env, sub string, args []string) Result {
	c, err := parseClusterFlags(sub, args)
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	client, err := newPlatformClient(c)
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	targetKind := c.kind
	switch strings.ToLower(c.kind) {
	case "kubernetes", "kubernetestarget":
		targetKind = "KubernetesTarget"
	case "edge", "edgeruntime", "edgeruntimetarget":
		targetKind = "EdgeRuntimeTarget"
	}
	spec := toolkit.IntentSpec{TargetKind: targetKind, OfferingID: c.offering, TargetID: c.targetID, DisplayName: c.displayName}
	if sub == "import" {
		if c.credentialProvider == "" || c.credentialScope == "" || c.credentialName == "" || c.credentialVersion == "" {
			return fail(ReasonInvalidConfig, "import requires --credential-provider, --credential-scope, --credential-name and --credential-version")
		}
		spec.CredentialSecretRef = &toolkit.IntentSecretRef{Provider: c.credentialProvider, Scope: c.credentialScope, Name: c.credentialName, Version: c.credentialVersion}
	}
	kind := toolkit.IntentCreateKubernetesTarget
	if sub == "import" {
		kind = toolkit.IntentImportRuntimeTarget
	}
	requestCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	result, err := client.SubmitIntent(requestCtx, kind, c.idemKey, c.corrID, spec)
	if err != nil {
		return platformResult(err)
	}
	if c.format == "json" {
		if err := json.NewEncoder(env.Stdout).Encode(result); err != nil {
			return fail(ReasonOutputFailed, "cannot write intent result")
		}
	} else {
		fmt.Fprintf(env.Stdout, "intent=%s status=%s operation=%s replayed=%v\n", result.IntentID, result.Status, result.OperationID, result.Replayed)
	}
	return success("")
}

func runOperation(ctx context.Context, env *Env, args []string) Result {
	// Accept both `operation get <id> --flags` and `operation get --operation <id>`.
	var positional []string
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			positional = append(positional, a)
		}
	}
	if len(positional) == 0 {
		return fail(ReasonInvalidConfig, "operation get <operationId> required")
	}
	if positional[0] != "get" {
		return fail(ReasonInvalidConfig, "only `operation get` is supported")
	}
	operationID := ""
	if len(positional) > 1 {
		operationID = positional[1]
	}
	flagArgs := make([]string, 0, len(args))
	for _, a := range args {
		if a == positional[0] {
			continue
		}
		if operationID != "" && a == operationID {
			continue
		}
		flagArgs = append(flagArgs, a)
	}
	c, err := parseClusterFlags("operation get", flagArgs)
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	if c.operation == "" {
		c.operation = operationID
	}
	if c.operation == "" {
		return fail(ReasonInvalidConfig, "operation get <operationId> required")
	}
	client, err := newPlatformClient(c)
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	requestCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	ref, err := client.GetOperation(requestCtx, c.operation)
	if err != nil {
		return platformResult(err)
	}
	if c.format == "json" {
		if err := json.NewEncoder(env.Stdout).Encode(ref); err != nil {
			return fail(ReasonOutputFailed, "cannot write operation result")
		}
	} else {
		fmt.Fprintf(env.Stdout, "operation=%s status=%s\n", ref.OperationID, ref.Status)
	}
	return success("")
}
