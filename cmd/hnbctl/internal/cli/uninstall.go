package cli

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"
)

// runUninstall is the concise uninstall entry. It resolves the recorded install
// run (by --run-dir, or the latest run under --runs-dir), rebuilds the strict
// ownership manifest from the config that run recorded, and produces a reviewable
// removal plan. It never executes deletion: removal remains a server-authoritative
// operation (platform uninstall-hnb/unmanage/delete-management-base).
func runUninstall(ctx context.Context, env *Env, args []string) Result {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var runDir, runsDir, runID, installationID, configPath, mode, format, ownedBaseUID, kubeconfig, instanceLabel, namespace string
	var acknowledgeStorage, installCreatedBase, deleteData, yes, planOnly, execute, all, data bool
	fs.StringVar(&runDir, "run-dir", "", "recorded install run directory")
	fs.StringVar(&runsDir, "runs-dir", "", "install history root (default: XDG state home/hnbctl/runs)")
	fs.StringVar(&runID, "run-id", "", "install run id (default: latest under --runs-dir)")
	fs.StringVar(&installationID, "installation", "", "installation id; restores the saved configuration without -f")
	fs.StringVar(&configPath, "f", "", "DeploymentConfig of the recorded install run")
	fs.StringVar(&mode, "mode", "uninstall-hnb", "uninstall-hnb|unmanage|delete-management-base")
	fs.StringVar(&format, "format", "human", "stdout format: human or json")
	fs.BoolVar(&acknowledgeStorage, "acknowledge-storage-removal", false, "confirm removal of persistent storage owned by this release")
	fs.BoolVar(&deleteData, "delete-data", false, "include this installation's persistent data (default: retain)")
	fs.BoolVar(&yes, "yes", false, "confirm data removal for automation; never bypass ownership/execution gates")
	fs.BoolVar(&planOnly, "plan", false, "only output the removal plan; no deletion")
	fs.BoolVar(&execute, "execute", false, "execute the reviewed uninstall-hnb plan (retains PVC/Secret/namespace)")
	fs.BoolVar(&installCreatedBase, "install-created-base", false, "assert this install created the management base")
	fs.StringVar(&ownedBaseUID, "owned-base-uid", "", "observed management-base UID (required for delete-management-base)")
	fs.StringVar(&kubeconfig, "kubeconfig", "", "override kubeconfig used for execution")
	fs.StringVar(&instanceLabel, "instance-label", "hnb", "app.kubernetes.io/instance label for cluster-scoped RBAC")
	fs.StringVar(&namespace, "namespace", "hnb-system", "namespace used by zero-config auto-detection")
	fs.BoolVar(&all, "all", false, "auto mode: also remove TopoLVM/StorageClass/CSI components")
	fs.BoolVar(&data, "data", false, "auto mode: also remove PVC/PV/LogicalVolume/Secret/namespace data")
	if err := fs.Parse(args); err != nil {
		return fail(ReasonInvalidConfig, "invalid uninstall flags")
	}
	if fs.NArg() != 0 {
		return fail(ReasonInvalidConfig, "unexpected uninstall arguments")
	}
	var emptyInstallation bool
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "installation" && installationID == "" {
			emptyInstallation = true
		}
	})
	if emptyInstallation {
		return fail(ReasonInvalidConfig, "--installation must name a recorded installation")
	}
	if installationID != "" {
		if runID != "" || runDir != "" {
			return fail(ReasonInvalidConfig, "choose --installation or --run-id/--run-dir")
		}
		runID = installationID
	}
	if deleteData && mode != string(toolkit.RemovalUninstallHNB) {
		return fail(ReasonInvalidConfig, "--delete-data is only supported for uninstall-hnb")
	}
	if format != "human" && format != "json" {
		return fail(ReasonInvalidConfig, "--format must be human or json")
	}
	if data {
		deleteData = true
	}
	legacy := runDir != "" || runsDir != "" || runID != "" || installationID != "" || configPath != ""
	if all && legacy {
		return fail(ReasonInvalidConfig, "--all is only valid in zero-config mode; omit --run-dir/--installation/-f")
	}
	if !legacy {
		return runAutoUninstall(ctx, env, namespace, kubeconfig, all, deleteData, yes, planOnly, format)
	}
	if runDir != "" && (runsDir != "" || runID != "") {
		return fail(ReasonInvalidConfig, "provide --run-dir or --runs-dir/--run-id, not both")
	}
	dir := runDir
	if dir == "" {
		var err error
		dir, err = resolveRunDir("", runsDir, runID)
		if err != nil {
			return fail(ReasonInvalidConfig, err.Error())
		}
	}
	run, err := toolkit.LoadRunState(dir)
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	if runID != "" && run.RunID != runID {
		return fail(ReasonInvalidConfig, "stored run id does not match --run-id")
	}
	var cfg *toolkit.DeploymentConfig
	if configPath == "" {
		cfg, err = toolkit.LoadInstallSnapshot(dir)
	} else {
		cfg, err = readConfig(configPath)
	}
	if err != nil {
		return fail(ReasonInvalidConfig, "cannot restore installation configuration: "+err.Error())
	}
	if problems := cfg.Validate(toolkit.CurrentEvidence); len(problems) > 0 {
		return fail(ReasonInvalidConfig, strings.Join(problems, "; "))
	}
	if cfg.Spec.Mode != toolkit.ModeExisting {
		return fail(ReasonTargetActionMissing, "bootstrap removal requires a recorded owned-resource inventory and observed management-base identity; a host installation plan is not ownership evidence")
	}
	if mode == string(toolkit.RemovalDeleteBase) {
		return fail(ReasonTargetActionMissing, "this installation targets an external cluster; its management base is not owned by this install")
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
	wantsData := deleteData || acknowledgeStorage
	plan, err := toolkit.PlanRemoval(toolkit.RemovalInput{Manifest: manifest, Run: run,
		Mode: toolkit.RemovalMode(mode), InstallCreatedBase: installCreatedBase,
		OwnedBaseUID: ownedBaseUID, DataLossAcknowledged: acknowledgeStorage || (deleteData && yes),
		RetainData: !wantsData, RetainNamespace: !deleteData, PurgeData: deleteData})
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	if deleteData && !acknowledgeStorage && !yes && !planOnly {
		toolkit.FormatRemovalHuman(env.Stderr, plan)
		if !confirmOperation(env, "Remove persistent data for installation "+run.RunID+" in namespace "+manifest.Namespace+"?") {
			return fail(ReasonPreflightBlocked, "data removal not confirmed; no changes made (use --yes for automation or --plan to inspect)")
		}
		// Recompute the entire plan after confirmation; confirmation never clears
		// unrelated ownership or management-base blocks.
		plan, err = toolkit.PlanRemoval(toolkit.RemovalInput{Manifest: manifest, Run: run,
			Mode: toolkit.RemovalMode(mode), InstallCreatedBase: installCreatedBase,
			OwnedBaseUID: ownedBaseUID, DataLossAcknowledged: true, RetainData: false, RetainNamespace: !deleteData, PurgeData: deleteData})
		if err != nil {
			return fail(ReasonInvalidConfig, err.Error())
		}
	}
	if !execute {
		if format == "json" {
			if err := json.NewEncoder(env.Stdout).Encode(plan); err != nil {
				return fail(ReasonOutputFailed, "cannot write removal plan")
			}
		} else {
			toolkit.FormatRemovalHuman(env.Stdout, plan)
		}
	}
	if planOnly {
		return success("")
	}
	if !plan.Authorized {
		return fail(ReasonTargetActionMissing, "removal blocked: "+strings.Join(plan.Blocked, "; "))
	}
	if !execute {
		return fail(ReasonPlatformNotVerified, "removal plan authorized; pass --execute --yes to delete HNB workloads (PVC/Secret/namespace retained unless --delete-data)")
	}
	return executeReviewedRemoval(ctx, env, cfg, plan, format, kubeconfig, instanceLabel, yes, deleteData)
}

func executeReviewedRemoval(ctx context.Context, env *Env, cfg *toolkit.DeploymentConfig, plan toolkit.RemovalPlan, format, kubeconfig, instanceLabel string, yes, deleteData bool) Result {
	if !yes {
		toolkit.FormatRemovalHuman(env.Stderr, plan)
		message := "Delete HNB workloads in namespace " + plan.Namespace + "? PVCs, Secrets and the namespace will be retained."
		if deleteData {
			message = "DELETE ALL DATA in namespace " + plan.Namespace + "? PVCs, PVs, Secrets and the namespace will be permanently removed."
		}
		if !confirmOperation(env, message) {
			return fail(ReasonPreflightBlocked, "removal not confirmed; no changes made (use --yes for automation)")
		}
	}
	if kubeconfig == "" {
		kubeconfig = cfg.Spec.Cluster.KubeconfigFile
	}
	selector := "app.kubernetes.io/instance=" + strings.TrimSpace(instanceLabel)
	if selector == "app.kubernetes.io/instance=" {
		selector = "app.kubernetes.io/instance=hnb"
	}
	executor := toolkit.RemovalExecutor{Kubeconfig: kubeconfig, Namespace: plan.Namespace, Selector: selector, DeleteData: deleteData}
	result, err := executor.Execute(ctx, plan)
	if err != nil {
		return fail(ReasonPlatformNotVerified, err.Error())
	}
	if format == "json" {
		if err := json.NewEncoder(env.Stdout).Encode(result); err != nil {
			return fail(ReasonOutputFailed, "cannot write removal result")
		}
	} else {
		fmt.Fprintf(env.Stdout, "uninstalled HNB workloads in %s; retained %d resource groups; verified=%v\n", plan.Namespace, len(result.Retained), result.Verified)
	}
	return success("")
}
