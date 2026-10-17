package cli

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"
)

// Command is one hnbctl subcommand.
type Command struct {
	Name        string
	Usage       string
	Summary     string
	PlanStage   string // "implemented" | "planned" (design contract, not shipped yet)
	Unsupported bool   // server-side action not yet certified
	Run         func(ctx context.Context, env *Env, args []string) Result
}

// Env carries the command context: output streams and the release lock path.
type Env struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// Commands is the registry. Planned interfaces are present so help is explicit
// and unsupported actions return TARGET_ACTION_UNSUPPORTED instead of printing
// fake install instructions or producing no-op operations.
var Commands []Command

func init() {
	Commands = []Command{
		{Name: "os", Summary: "list OS adapters or inspect this host (read-only)", Usage: "hnbctl os [list|inspect] [--format human|json]", Run: runOS},
		{Name: "render", Summary: "render a locked candidate Chart with Secret references (no cluster writes)", Usage: "hnbctl render -f hnb.yaml --chart-package hnb.tgz --bindings bindings.yaml --output rendered.yaml [--stage startup|full] [--lock release.lock.yaml]", Run: runRender},
		{Name: "recovery", Summary: "verify original install and backup input identities (read-only; no restore)", Usage: "hnbctl recovery verify -f hnb.yaml --receipt receipt.json --backup-manifest backup.json --backup-dir <dir> [--lock <lock>] [--format human|json]", Run: runRecovery},
		{Name: "bundle", Summary: "prepare/verify/import a verified offline dependency closure", Usage: "hnbctl bundle prepare|verify|import --lock <lock> --cache-dir <dir> --manifest <bundle.json> [--target-dir <import-cache>] [--os-package-index <f> --sandbox-image <f>]", Run: runBundle},
		{Name: "capacity", Summary: "estimate capacity from rendered requests (planning only)", Usage: "hnbctl capacity --profile Minimal --manifest rendered.yaml --nodes capacity-nodes.yaml [--evidence hnb.yaml] [--material-verified]", Run: runCapacity},
		{Name: "recipe", Summary: "show the node dependency declaration derived from the provision scripts", Usage: "hnbctl recipe show -f hnb.yaml --os ubuntu-24.04 --image-repository <repo> --sandbox-image <ref> --cni-image-repository <repo>", Run: runRecipe},
		{Name: "lock", Summary: "generate an artifact-identified candidate release lock", Usage: "hnbctl lock generate -f release-input.yaml --output release.lock.yaml", Run: runLock},
		{Name: "identity", Summary: "generate instance identity, KMS material and bootstrap admin credential", Usage: "hnbctl identity generate --issuer https://issuer.hnb.local/ --audience hnb-apiserver --entry https://portal.hnb.local --dir /var/lib/hnb/identity", Run: runIdentity},
		{
			Name: "version", Summary: "print the hnbctl version and support matrix digest",
			Usage: "hnbctl version",
			Run: func(_ context.Context, env *Env, _ []string) Result {
				fmt.Fprintln(env.Stdout, VersionString)
				return success("")
			},
		},
		{
			Name: "help", Summary: "show command help",
			Usage: "hnbctl help [command]",
			Run: func(_ context.Context, env *Env, args []string) Result {
				if len(args) > 0 {
					for _, c := range Commands {
						if c.Name == args[0] {
							printCommand(env.Stdout, c)
							return success("")
						}
					}
					return failf(ReasonInvalidConfig, "unknown command %q", args[0])
				}
				printAll(env.Stdout)
				return success("")
			},
		},
		{
			Name: "matrix", Summary: "print the certified capability and evidence matrix",
			Usage: "hnbctl matrix",
			Run:   runMatrix,
		},
		{
			Name: "create", Summary: "create a DeploymentConfig draft (kk-style: like init)",
			Usage: "hnbctl create config -f hnb.yaml [--mode existing|bootstrap] [--os NAME]",
			Run:   runCreate,
		},
		{
			Name: "add", Summary: "add nodes to a managed runtime target (kk-style: like node add)",
			Usage: "hnbctl add nodes -f nodes.yaml --target-id ID --platform-url URL --token-file PATH",
			Run:   runAdd,
		},
		{
			Name: "delete", Summary: "plan removal of an installation or management base (kk-style)",
			Usage: "hnbctl delete installation --installation ID [flags]\n           hnbctl delete cluster ...",
			Run:   runDelete,
		},
		{
			Name: "upgrade", Summary: "plan a profile or pinned platform upgrade (kk-style)",
			Usage: "hnbctl upgrade --to LiteHA|StandardHA|Enterprise [-f hnb.yaml] [--allow-downgrade] [--backing bundled|in-cluster-ha|external] [--control-node-count N] [--operator-ready true|false]\n    or: hnbctl upgrade --current-lock A --release-lock B [--db-compatible] [--api-reachable]",
			Run:   runUpgrade,
		},
		{
			Name: "scale", Summary: "plan on-demand horizontal/vertical/storage scaling with guardrails",
			Usage: "hnbctl scale --component NAME --replicas N [-f hnb.yaml] [--storage NAME=Gi]... [--component postgresql|nats for the in-cluster data plane] [--control-node-count N] [--operator-ready true|false] [--execute]",
			Run:   runScale,
		},
		{
			Name: "artifact", Summary: "manage verified offline dependency bundles (kk-style: like bundle)",
			Usage: "hnbctl artifact prepare|verify|import --lock L --cache-dir D --manifest M [--target-dir T]",
			Run:   runArtifact,
		},
		{
			Name: "completion", Summary: "generate shell completion for bash/zsh/fish",
			Usage: "hnbctl completion bash|zsh|fish",
			Run:   runCompletion,
		},
		{
			Name: "init", Summary: "create a DeploymentConfig draft",
			Usage: "hnbctl init [--interactive] --mode bootstrap|existing --profile Minimal [--os <OS from hnbctl os>] -f hnb.yaml",
			Run:   runInit,
		},
		{
			Name: "validate", Summary: "validate a DeploymentConfig and ReleaseLock",
			Usage: "hnbctl validate -f hnb.yaml [--lock release.lock.yaml]",
			Run:   runValidate,
		},
		{
			Name: "ssh", Summary: "explicitly enroll a dedicated SSH public key using password authentication",
			Usage: "hnbctl ssh enroll -f hnb.yaml --node NAME --key-file PATH --ask-pass [--yes]\n       Password file: --ssh-password-file PATH; preflight also accepts --ask-sudo-pass or --sudo-password-file PATH",
			Run:   runSSH,
		},
		{
			Name: "preflight", Summary: "run the read-only preflight (no side effects)",
			Usage: "hnbctl preflight -f hnb.yaml [--ask-pass | --ssh-password-file PATH] [--ask-sudo-pass | --sudo-password-file PATH] [--format human|json] [--output report.json]",
			Run:   runPreflight,
		},
		{
			Name: "prepare", Summary: "download/verify/cache locked artifacts",
			Usage: "hnbctl prepare -f hnb.yaml --cache-dir <dir> [--source-dir <release-package>] [--mirror-origin https://registry]",
			Run:   runPrepare,
		},
		{
			Name: "handoff", Summary: "establish the first management relationship via formal registration + live agent observation",
			Usage: "hnbctl handoff --run-dir <dir> --target-id <id> --idempotency-key <key> --platform-url https://... --token-file <path>",
			Run:   runHandoff,
		},
		{
			Name: "install", Summary: "run the certified install route",
			Usage: "hnbctl install (-f hnb.yaml | --host IP --user ubuntu --ssh-key PATH | --kubeconfig PATH)\n    Shortcut: --domain HOST --tls-secret NAME --storage-class NAME [--profile Minimal] [--lock PATH]\n    Authentication: --ask-pass | --ssh-password-file PATH; --ask-sudo-pass | --sudo-password-file PATH\n    [--internal-address IP] [--namespace hnb-system] [--name hnb] [--runs-dir DIR] [--plan] [--yes] [--format human|json] [--output plan.json]\n    Backing: --backing bundled|in-cluster-ha|external [--control-node-count N]\n    Execute (gated): --execute [--chart-package PATH --bindings PATH [--workload Kind/name]... [--migrations-dir DIR]]  (existing cluster)\n    Bootstrap execute: --image-repository REF --sandbox-image REF --cni-image-repository REF --chart-repository URL  (blank host)\n    Currently records plans and runs shortcut preflight; --execute enforces the delivery evidence gate and, when certified, applies the rendered release (existing) or runs the Day-0 management bootstrap (blank host).",
			Run:   runInstall,
		},
		{
			Name: "status", Summary: "show install progress for a run (defaults to the latest recorded run)",
			Usage: "hnbctl status [--run-dir <dir> | --runs-dir <dir> --run-id <id>] [--watch] [--interval <dur>] [--format human|json]",
			Run:   runStatus,
		},
		{
			Name: "resume", Summary: "resume a failed/blocked install after revalidation",
			Usage: "hnbctl resume --run-dir <dir> [--run-id <runId>] [--format human|json]",
			Run:   runResume,
		},
		{
			Name: "history", Summary: "list recorded install runs, newest first",
			Usage: "hnbctl history [--runs-dir <dir>] [--limit <n>] [--format human|json]",
			Run:   runHistory,
		},
		{
			Name: "logs", Summary: "read or follow a run's state-transition log",
			Usage: "hnbctl logs [--run-dir <dir> | --runs-dir <dir> --run-id <id>] [--tail <n>] [--follow] [--interval <dur>] [--format human|json]",
			Run:   runLogs,
		},
		{
			Name: "uninstall", Summary: "plan removal of a recorded install run's owned resources",
			Usage: "hnbctl uninstall --installation ID [--runs-dir DIR] [--plan] [--execute --yes] [--format human|json]\n    Also: [--run-dir DIR | --runs-dir DIR --run-id ID] [-f hnb.yaml] [--mode uninstall-hnb|unmanage|delete-management-base]\n    Default plan only. --execute --yes deletes HNB workloads/RBAC in the namespace and retains PVC/Secret/namespace/StorageClass.",
			Run:   runUninstall,
		},
		{
			Name: "verify", Summary: "run layered node/cluster/platform/managed checks",
			Usage: "hnbctl verify [--level node|cluster|platform|managed] [--format human|json]",
			Run:   runVerify,
		},
		{
			Name: "collect", Summary: "export a redacted diagnostics archive",
			Usage: "hnbctl collect --run-dir <dir> --output diagnostics.tar.gz [--reason-codes a,b]",
			Run:   runCollect,
		},
		{
			Name: "cluster", Summary: "managed cluster operations (server-authoritative)",
			Usage: "hnbctl cluster create|import|list\n    list: --platform-url https://... --token-file <path> [--format human|json]",
			Run:   runCluster,
		},
		{
			Name: "node", Summary: "node operations (server-authoritative)",
			Usage: "hnbctl node add --target-id <targetId> --expected-version <version> -f nodes.yaml --platform-url https://... --token-file <path> --idempotency-key <key> --correlation-id <uuid> [--tenant-scope <scope>] [--format human|json]",
			Run:   runNode,
		},
		{
			Name: "platform", Summary: "platform upgrade/rollback/uninstall (server-authoritative)",
			Usage: "hnbctl platform uninstall-hnb|unmanage|delete-management-base --run-dir <dir> -f hnb.yaml [--execute --yes]\n       hnbctl platform upgrade|rollback --current-lock <a> --release-lock <b> --api-reachable --db-compatible",
			Run:   runPlatform,
		},
		{
			Name: "operation", Summary: "query or act on an Operation",
			Usage: "hnbctl operation get <operationId>",
			Run:   runOperation,
		},
		{
			Name: "release", Summary: "owning-plane release material directory (register/list require the platform; verify is offline)",
			Usage: "hnbctl release register|list|verify\n    register: --bundle <bundle.json> --lock <lock.yaml> --cache-dir <dir> --role console|control-plane|worker --platform-url https://... --token-file <path> [--timeout <dur>] [--format human|json]\n    list:     --platform-url https://... --token-file <path> [--format human|json]\n    verify:   --bundle <bundle.json> --lock <lock.yaml> --cache-dir <dir> --role console|control-plane|worker [--format human|json] (refuses platform flags)",
			Run:   runRelease,
		},
	}
	sort.Slice(Commands, func(i, j int) bool { return Commands[i].Name < Commands[j].Name })
}

func printAll(w io.Writer) {
	fmt.Fprintln(w, "HNB deployment toolkit (hnbctl)")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Usage: hnbctl <command> [flags]")
	fmt.Fprintln(w, "")
	for _, c := range Commands {
		marker := ""
		if c.Unsupported {
			marker = " [unsupported]"
		}
		fmt.Fprintf(w, "  %-12s %s%s\n", c.Name, c.Summary, marker)
	}
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Unsupported actions return exit code 3 (TARGET_ACTION_UNSUPPORTED).")
}

func printCommand(w io.Writer, c Command) {
	marker := ""
	if c.Unsupported {
		marker = " [unsupported]"
	}
	fmt.Fprintf(w, "Usage: %s%s\n\n%s\n", c.Usage, marker, c.Summary)
}

// Execute dispatches argv and returns the process exit code.
func Execute(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return ExecuteWithInput(ctx, args, nil, stdout, stderr)
}

func ExecuteWithInput(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	env := &Env{Stdin: stdin, Stdout: stdout, Stderr: stderr}
	if len(args) == 0 {
		printAll(stdout)
		return ExitUsage
	}
	name := args[0]
	if len(args) == 1 && (name == "--help" || name == "-h") {
		printAll(stdout)
		return ExitSuccess
	}
	if len(args) == 1 && name == "--version" {
		name = "version"
	}
	for _, c := range Commands {
		if c.Name == name {
			// Keep existing `help <command>` while accepting the familiar flag form.
			if (len(args) == 2 || (len(args) == 3 && !strings.HasPrefix(args[1], "-"))) &&
				(args[len(args)-1] == "--help" || args[len(args)-1] == "-h") {
				printCommand(stdout, c)
				return ExitSuccess
			}
			res := c.Run(ctx, env, args[1:])
			if res.Message != "" {
				out := env.Stdout
				if res.ExitCode != ExitSuccess {
					out = env.Stderr
				}
				if res.ExitCode != ExitSuccess && res.Reason != "" {
					fmt.Fprintf(out, "%s: %s\n", res.Reason, toolkit.SafeReportText(res.Message))
				} else {
					fmt.Fprintln(out, toolkit.SafeReportText(res.Message))
				}
			}
			return res.ExitCode
		}
	}
	fmt.Fprintf(stderr, "unknown command %q (see: hnbctl help)\n", name)
	return ExitUsage
}
