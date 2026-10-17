package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"
)

// runPlatform exposes the server-authoritative platform actions plus the three
// separate removal entry points required by PINST-007.
func runPlatform(ctx context.Context, env *Env, args []string) Result {
	if len(args) == 0 {
		return fail(ReasonInvalidConfig, "platform subcommand required: uninstall-hnb|unmanage|delete-management-base")
	}
	switch args[0] {
	case "uninstall-hnb", "unmanage", "delete-management-base":
		return runRemoval(ctx, env, args[0], args[1:])
	case "upgrade":
		return runChange(env, "upgrade", args[1:])
	case "rollback":
		return runChange(env, "resource-rollback", args[1:])
	default:
		return fail(ReasonInvalidConfig, "unknown platform subcommand: "+args[0])
	}
}

// runChange derives a pinned platform change plan from two validated release
// locks. Compatibility and backup availability are caller-declared observed
// facts; they are never assumed.
func runChange(env *Env, direction string, args []string) Result {
	fs := flag.NewFlagSet("platform "+direction, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var currentPath, targetPath, format, backupDigest string
	var dbCompatible, apiReachable bool
	fs.StringVar(&currentPath, "current-lock", "", "release lock currently installed")
	fs.StringVar(&targetPath, "release-lock", "", "target release lock (pinned by content digest)")
	fs.StringVar(&format, "format", "human", "stdout format: human or json")
	fs.BoolVar(&dbCompatible, "db-compatible", false, "declare schema compatibility with the target release was verified")
	fs.BoolVar(&apiReachable, "api-reachable", false, "declare the platform API answered before requesting the change")
	fs.StringVar(&backupDigest, "backup-digest", "", "digest of an available, verified backup")
	if err := fs.Parse(args); err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	if fs.NArg() != 0 {
		return fail(ReasonInvalidConfig, "unexpected arguments")
	}
	if format != "human" && format != "json" {
		return fail(ReasonInvalidConfig, "--format must be human or json")
	}
	if currentPath == "" || targetPath == "" {
		return fail(ReasonInvalidConfig, "--current-lock and --release-lock are required")
	}
	// A generic target change may never be requested while the platform API is
	// unreachable: there is no local execution fallback (PINST-006 / DPLY-006).
	if err := toolkit.AuthorizeGenericTargetChange(apiReachable); err != nil {
		return platformResult(err)
	}
	current, err := readValidatedStandaloneLock(currentPath)
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	target, err := readValidatedStandaloneLock(targetPath)
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	plan, err := toolkit.BuildPlatformChangePlan(direction, current, target,
		toolkit.LockContentDigest(current), dbCompatible, backupDigest, backupDigest, time.Now().UTC())
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	if format == "json" {
		if err := json.NewEncoder(env.Stdout).Encode(plan); err != nil {
			return fail(ReasonOutputFailed, "cannot write platform change plan")
		}
	} else {
		toolkit.FormatPlatformChangeHuman(env.Stdout, plan)
	}
	if !plan.Authorized {
		return fail(ReasonTargetActionMissing, "platform change blocked: "+strings.Join(plan.Blocked, "; "))
	}
	// Authorized means the plan is complete, not that the change was applied.
	return fail(ReasonPlatformNotVerified, "platform change plan authorized; execution runs through the formal Operation contract")
}

// runRemoval produces a reviewable removal plan. uninstall-hnb can be executed
// locally with --execute; unmanage/delete-management-base remain server actions.
func runRemoval(ctx context.Context, env *Env, mode string, args []string) Result {
	fs := flag.NewFlagSet("platform "+mode, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var runDir, configPath, format, kubeconfig, instanceLabel string
	var acknowledgeStorage, installCreatedBase, execute, yes bool
	var ownedBaseUID string
	fs.StringVar(&runDir, "run-dir", "", "private install run directory")
	fs.StringVar(&configPath, "f", "", "DeploymentConfig of the recorded install run")
	fs.StringVar(&format, "format", "human", "stdout format: human or json")
	fs.BoolVar(&acknowledgeStorage, "acknowledge-storage-removal", false, "confirm removal of persistent storage owned by this release")
	fs.BoolVar(&installCreatedBase, "install-created-base", false, "assert this install created the management base")
	fs.StringVar(&ownedBaseUID, "owned-base-uid", "", "observed management-base UID (required for delete-management-base)")
	fs.BoolVar(&execute, "execute", false, "execute the reviewed uninstall-hnb plan")
	fs.BoolVar(&yes, "yes", false, "confirm execution for automation")
	fs.StringVar(&kubeconfig, "kubeconfig", "", "override kubeconfig used for execution")
	fs.StringVar(&instanceLabel, "instance-label", "hnb", "app.kubernetes.io/instance label for cluster-scoped RBAC")
	if err := fs.Parse(args); err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	if fs.NArg() != 0 {
		return fail(ReasonInvalidConfig, "unexpected arguments")
	}
	if format != "human" && format != "json" {
		return fail(ReasonInvalidConfig, "--format must be human or json")
	}
	if execute && acknowledgeStorage {
		return fail(ReasonInvalidConfig, "--execute currently retains data; omit --acknowledge-storage-removal")
	}
	if runDir == "" || configPath == "" {
		return fail(ReasonInvalidConfig, "--run-dir and -f are required")
	}
	run, err := toolkit.LoadRunState(runDir)
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	cfg, err := readConfig(configPath)
	if err != nil {
		return fail(ReasonInvalidConfig, "cannot read DeploymentConfig")
	}
	if problems := cfg.Validate(toolkit.CurrentEvidence); len(problems) > 0 {
		return fail(ReasonInvalidConfig, strings.Join(problems, "; "))
	}
	manifest, problems := cfg.Ownership()
	if len(problems) > 0 {
		return fail(ReasonInvalidConfig, strings.Join(problems, "; "))
	}
	// The removal plan must belong to the recorded run: recompute the config
	// identity the same way preflight does and require it to match.
	encoded, err := json.Marshal(cfg)
	if err != nil {
		return fail(ReasonInvalidConfig, "cannot compute configuration identity")
	}
	if digest := fmt.Sprintf("sha256:%x", sha256.Sum256(encoded)); digest != run.ConfigDigest {
		return fail(ReasonInvalidConfig, "DeploymentConfig does not match the recorded install run")
	}
	plan, err := toolkit.PlanRemoval(toolkit.RemovalInput{Manifest: manifest, Run: run,
		Mode: toolkit.RemovalMode(mode), InstallCreatedBase: installCreatedBase,
		OwnedBaseUID: ownedBaseUID, DataLossAcknowledged: acknowledgeStorage})
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	if execute {
		if !plan.Authorized {
			return fail(ReasonTargetActionMissing, "removal blocked: "+strings.Join(plan.Blocked, "; "))
		}
		return executeReviewedRemoval(ctx, env, cfg, plan, format, kubeconfig, instanceLabel, yes, false)
	}
	if format == "json" {
		if err := json.NewEncoder(env.Stdout).Encode(plan); err != nil {
			return fail(ReasonOutputFailed, "cannot write removal plan")
		}
	} else {
		toolkit.FormatRemovalHuman(env.Stdout, plan)
	}
	if !plan.Authorized {
		return fail(ReasonTargetActionMissing, "removal blocked: "+strings.Join(plan.Blocked, "; "))
	}
	return fail(ReasonPlatformNotVerified, "removal plan authorized; pass --execute --yes for local uninstall-hnb (PVC/Secret/namespace retained)")
}

// readValidatedStandaloneLock parses and validates a release lock that is not
// bound to a DeploymentConfig (a change plan compares two candidate releases).
func readValidatedStandaloneLock(path string) (*toolkit.ReleaseLock, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read lock: %w", err)
	}
	lock, err := toolkit.DecodeReleaseLock(data)
	if err != nil {
		return nil, fmt.Errorf("decode lock: %w", err)
	}
	if problems := lock.Validate(); len(problems) > 0 {
		return nil, fmt.Errorf("ReleaseLock is invalid: %s", strings.Join(problems, "; "))
	}
	return lock, nil
}

// fileDigest returns the pinned content identity and size of a local file.
func fileDigest(path string) (string, int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", 0, err
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), int64(len(data)), nil
}
