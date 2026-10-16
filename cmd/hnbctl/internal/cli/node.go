package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"

	"gopkg.in/yaml.v3"
)

// nodeAddDeclaration is the operator-authored -f file for `node add`. Only
// references and node identities cross the CLI boundary; raw secret material is
// never accepted.
type nodeAddDeclaration struct {
	SSHSecretRef toolkit.IntentSecretRef `yaml:"sshSecretRef"`
	Nodes        []nodeAddEntry          `yaml:"nodes"`
}

type nodeAddEntry struct {
	Name string `yaml:"name"`
	Role string `yaml:"role"`
}

// runNode dispatches the server-authoritative node subcommands.
func runNode(ctx context.Context, env *Env, args []string) Result {
	if len(args) == 0 {
		return fail(ReasonInvalidConfig, "node subcommand required: add")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "add":
		return runNodeAdd(ctx, env, rest)
	case "list", "status":
		// Per-target node listing lives in the console read model, not the
		// platform API this CLI is bound to. Refuse instead of fabricating rows.
		return fail(ReasonTargetActionMissing, "node "+sub+" is not available on the platform API; view managed targets with `hnbctl cluster list` and node-layer health with `hnbctl verify`")
	default:
		return fail(ReasonInvalidConfig, "unknown node subcommand: "+sub)
	}
}

// runNodeAdd submits an AddNodesRuntimeTarget intent. The platform decides the
// action (matrix/fencing/idempotency); the CLI never executes anything locally.
func runNodeAdd(ctx context.Context, env *Env, args []string) Result {
	fs := flag.NewFlagSet("node add", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var baseURL, tokenFile, tenant, idemKey, corrID, targetID, kind, nodesPath, format string
	var timeout time.Duration
	var expectedVersion int64
	fs.StringVar(&baseURL, "platform-url", "", "platform API base URL (HTTPS)")
	fs.StringVar(&tokenFile, "token-file", "", "file holding the API bearer token (never passed on the command line)")
	fs.StringVar(&tenant, "tenant-scope", "", "authenticated tenant scope for secret references")
	fs.StringVar(&idemKey, "idempotency-key", "", "stable idempotency key for this intent")
	fs.StringVar(&corrID, "correlation-id", "", "request correlation id (UUID)")
	fs.StringVar(&targetID, "target-id", "", "managed runtime target id")
	fs.StringVar(&targetID, "target", "", "managed runtime target id (alias of --target-id)")
	fs.Int64Var(&expectedVersion, "expected-version", 0, "current runtime target version (required)")
	fs.StringVar(&kind, "target-kind", "kubernetes", "target kind (kubernetes)")
	fs.StringVar(&nodesPath, "f", "", "node add declaration: nodes + sshSecretRef (YAML)")
	fs.StringVar(&format, "format", "human", "stdout format: human or json")
	fs.DurationVar(&timeout, "timeout", 30*time.Second, "platform request timeout")
	if err := fs.Parse(args); err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	if fs.NArg() != 0 {
		return fail(ReasonInvalidConfig, "unexpected arguments")
	}
	if targetID == "" {
		return fail(ReasonInvalidConfig, "node add requires --target-id")
	}
	if nodesPath == "" {
		return fail(ReasonInvalidConfig, "node add requires -f nodes.yaml")
	}
	declaration, err := readNodeAddDeclaration(nodesPath)
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	nodes, err := buildNodeParameters(declaration)
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	targetKind := "KubernetesTarget"
	if strings.ToLower(kind) != "kubernetes" {
		return fail(ReasonInvalidConfig, "node add supports only the kubernetes target kind")
	}
	if expectedVersion < 1 {
		return fail(ReasonInvalidConfig, "node add requires --expected-version >= 1")
	}
	if format != "human" && format != "json" {
		return fail(ReasonInvalidConfig, "--format must be human or json")
	}
	if timeout <= 0 {
		return fail(ReasonInvalidConfig, "--timeout must be positive")
	}
	client, err := newPlatformClient(clusterFlags{baseURL: baseURL, tokenFile: tokenFile, tenant: tenant, timeout: timeout})
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	spec := toolkit.IntentSpec{
		TargetID:        targetID,
		ExpectedVersion: expectedVersion,
		TargetKind:      targetKind,
		Parameters:      map[string]any{"nodes": nodes},
		SSHSecretRef:    &declaration.SSHSecretRef,
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result, err := client.SubmitIntent(requestCtx, toolkit.IntentAddNodesRuntimeTarget, idemKey, corrID, spec)
	if err != nil {
		return platformResult(err)
	}
	if format == "json" {
		if err := json.NewEncoder(env.Stdout).Encode(result); err != nil {
			return fail(ReasonOutputFailed, "cannot write intent result")
		}
	} else {
		fmt.Fprintf(env.Stdout, "intent=%s status=%s operation=%s replayed=%v\n", result.IntentID, result.Status, result.OperationID, result.Replayed)
	}
	return success("")
}

func readNodeAddDeclaration(path string) (nodeAddDeclaration, error) {
	var declaration nodeAddDeclaration
	data, err := os.ReadFile(path)
	if err != nil {
		return declaration, fmt.Errorf("read node add declaration: %v", err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&declaration); err != nil {
		return declaration, fmt.Errorf("invalid node add declaration: expected nodes and sshSecretRef only")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return declaration, fmt.Errorf("node add declaration must contain exactly one YAML document")
	}
	if declaration.SSHSecretRef.Provider == "" || declaration.SSHSecretRef.Name == "" {
		return declaration, fmt.Errorf("node add declaration requires sshSecretRef (provider/name)")
	}
	return declaration, nil
}

var nodeAddNamePattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)

// buildNodeParameters enforces the server-owned node-add payload shape locally
// (worker-only, 1..100 entries, RFC-1123 subdomain names, no duplicates) so an
// invalid request is refused before it leaves the installer.
func buildNodeParameters(declaration nodeAddDeclaration) ([]any, error) {
	if len(declaration.Nodes) == 0 {
		return nil, fmt.Errorf("node add declaration requires at least one node")
	}
	if len(declaration.Nodes) > 100 {
		return nil, fmt.Errorf("node add declaration supports at most 100 nodes")
	}
	seen := make(map[string]bool, len(declaration.Nodes))
	nodes := make([]any, 0, len(declaration.Nodes))
	for i, entry := range declaration.Nodes {
		if entry.Name == "" {
			return nil, fmt.Errorf("node %d has no name", i+1)
		}
		if len(entry.Name) > 253 {
			return nil, fmt.Errorf("node %q exceeds the 253 character name limit", entry.Name)
		}
		if !nodeAddNamePattern.MatchString(entry.Name) {
			return nil, fmt.Errorf("node %d must have an RFC-1123 subdomain name", i+1)
		}
		if entry.Role != "worker" {
			return nil, fmt.Errorf("node %q role %q is not supported; only worker nodes can be added", entry.Name, entry.Role)
		}
		if seen[entry.Name] {
			return nil, fmt.Errorf("duplicate node %q", entry.Name)
		}
		seen[entry.Name] = true
		nodes = append(nodes, map[string]any{"name": entry.Name, "role": entry.Role})
	}
	return nodes, nil
}
