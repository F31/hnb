package cli

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"
)

// runProfileUpgrade handles `hnbctl upgrade --to <profile>`: a side-effect-free
// profile transition plan (default). With --execute it runs the gated, resumable
// upgrade executor against the rendered target release.
func runProfileUpgrade(ctx context.Context, env *Env, args []string) Result {
	var to, from, configPath, lockPath, format, chartPackage, bindings, migrationsDir, runsDir string
	var backing, operatorReadyFlag string
	var controlNodeCount int
	var allowDowngrade, execute, backupVerified, allowIrreversible bool
	var workloads stringSlice
	fs := flag.NewFlagSet("upgrade", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&to, "to", "", "target profile: Minimal|LiteHA|StandardHA|Enterprise")
	fs.StringVar(&from, "from", "", "current profile (default: the config profile)")
	fs.StringVar(&configPath, "f", "", "DeploymentConfig")
	fs.StringVar(&lockPath, "lock", "", "ReleaseLock override")
	fs.StringVar(&backing, "backing", "", "data-plane backing override: bundled|in-cluster-ha|external")
	fs.IntVar(&controlNodeCount, "control-node-count", 0, "observed schedulable control-plane domains (>=3 for in-cluster-ha; 0 = unknown)")
	fs.StringVar(&operatorReadyFlag, "operator-ready", "", "observed data-plane operator readiness: true|false (omit if unknown)")
	fs.BoolVar(&allowDowngrade, "allow-downgrade", false, "acknowledge a level-lowering change")
	fs.BoolVar(&execute, "execute", false, "execute the plan (requires certification and applier inputs)")
	fs.StringVar(&chartPackage, "chart-package", "", "verified chart package to render for the target profile (--execute)")
	fs.StringVar(&bindings, "bindings", "", "render bindings file for --execute")
	fs.Var(&workloads, "workload", "workload to wait for rollout as Kind/name (repeatable; --execute)")
	fs.StringVar(&migrationsDir, "migrations-dir", "", "prepared migration material root (--execute)")
	fs.StringVar(&runsDir, "runs-dir", "", "private upgrade history root (--execute)")
	fs.BoolVar(&backupVerified, "backup-verified", false, "assert a verified backup exists (required when the plan migrates)")
	fs.BoolVar(&allowIrreversible, "allow-irreversible", false, "acknowledge irreversible migrations")
	fs.StringVar(&format, "format", "human", "human or json")
	if err := fs.Parse(args); err != nil {
		return fail(ReasonInvalidConfig, "invalid upgrade flags: "+err.Error())
	}
	if fs.NArg() != 0 {
		return fail(ReasonInvalidConfig, "unexpected upgrade arguments")
	}
	if to == "" {
		return fail(ReasonInvalidConfig, "--to is required (Minimal|LiteHA|StandardHA|Enterprise)")
	}
	if format != "human" && format != "json" {
		return fail(ReasonInvalidConfig, "--format must be human or json")
	}
	if backing != "" && backing != "bundled" && backing != "in-cluster-ha" && backing != "external" {
		return fail(ReasonInvalidConfig, "--backing must be bundled, in-cluster-ha or external")
	}
	if controlNodeCount < 0 {
		return fail(ReasonInvalidConfig, "--control-node-count must not be negative")
	}
	if backing == "in-cluster-ha" && controlNodeCount > 0 && controlNodeCount < 3 {
		return fail(ReasonInvalidConfig, "HA_CONTROL_NODES_INSUFFICIENT: in-cluster-ha requires at least 3 schedulable control-plane domains")
	}
	operatorReady, opRes := parseOperatorReady(operatorReadyFlag)
	if opRes.ExitCode != ExitSuccess {
		return opRes
	}
	target := toolkit.Profile(to)
	if _, ok := toolkit.ProfileShape(target); !ok {
		return fail(ReasonInvalidConfig, "unknown target profile "+to)
	}
	if from == "" {
		if configPath == "" {
			return fail(ReasonInvalidConfig, "provide -f or --from to name the current profile")
		}
		cfg, err := readConfig(configPath)
		if err != nil {
			return failf(ReasonInvalidConfig, "cannot read DeploymentConfig: %v", err)
		}
		from = string(cfg.Spec.Profile)
		if lockPath == "" {
			lockPath = cfg.Spec.ReleaseLock
		}
	}
	source := toolkit.Profile(from)
	if _, ok := toolkit.ProfileShape(source); !ok {
		return fail(ReasonInvalidConfig, "unknown source profile "+from)
	}
	var lock *toolkit.ReleaseLock
	if lockPath != "" {
		data, err := os.ReadFile(lockPath)
		if err == nil {
			decoded, derr := toolkit.DecodeReleaseLock(data)
			if derr == nil {
				lock = decoded
			}
		}
	}
	plan, err := toolkit.PlanUpgrade(toolkit.UpgradeInput{
		From: source, To: target, AllowDowngrade: allowDowngrade,
		Lock: lock, Evidence: toolkit.CurrentEvidence,
		BackingOverride: backing, ControlPlaneDomains: controlNodeCount, OperatorReady: operatorReady,
	})
	if err != nil {
		return failf(ReasonInvalidConfig, "cannot plan upgrade: %v", err)
	}
	if format == "json" {
		if err := json.NewEncoder(env.Stdout).Encode(plan); err != nil {
			return fail(ReasonOutputFailed, "cannot write upgrade plan")
		}
	} else {
		fmt.Fprintf(env.Stdout, "upgrade %s -> %s authorized=%v\n", plan.From, plan.To, plan.Authorized)
		for _, step := range plan.Steps {
			reversibility := "reversible"
			if !step.Reversible {
				reversibility = "irreversible"
			}
			fmt.Fprintf(env.Stdout, "  %s %s %s (%s)\n", step.Kind, step.Target, step.Detail, reversibility)
		}
		for _, reason := range plan.Blocked {
			fmt.Fprintf(env.Stdout, "  blocked: %s\n", reason)
		}
	}
	if execute {
		if !plan.Authorized {
			return fail(ReasonPlatformNotVerified, "upgrade execution refused: "+strings.Join(plan.Blocked, "; "))
		}
		return runProfileUpgradeExecution(ctx, env, plan, configPath, lock, chartPackage, bindings, workloads, migrationsDir, runsDir, backupVerified, allowIrreversible)
	}
	return success("")
}

// runProfileUpgradeExecution renders the target release and drives the gated
// upgrade executor. It is only reached once PlanUpgrade is authorized.
func runProfileUpgradeExecution(ctx context.Context, env *Env, plan toolkit.UpgradePlan, configPath string, lock *toolkit.ReleaseLock, chartPackage, bindings string, workloads stringSlice, migrationsDir, runsDir string, backupVerified, allowIrreversible bool) Result {
	if configPath == "" {
		return fail(ReasonInvalidConfig, "--execute requires -f to load the target configuration")
	}
	if chartPackage == "" || bindings == "" {
		return fail(ReasonInvalidConfig, "upgrade execution requires --chart-package and --bindings")
	}
	if lock == nil {
		return fail(ReasonInvalidConfig, "upgrade execution requires a release lock (--lock or spec.releaseLock)")
	}
	cfg, err := readConfig(configPath)
	if err != nil {
		return failf(ReasonInvalidConfig, "cannot read DeploymentConfig: %v", err)
	}
	if problems := cfg.Validate(toolkit.CurrentEvidence); len(problems) > 0 {
		return failf(ReasonInvalidConfig, "%s", strings.Join(problems, "; "))
	}
	bindingBytes, err := os.ReadFile(bindings)
	if err != nil {
		return failf(ReasonInvalidConfig, "cannot read bindings: %v", err)
	}
	rendered, err := toolkit.RenderLockedChart(ctx, cfg, lock, chartPackage, bindingBytes, "full")
	if err != nil {
		return failf(ReasonPlatformNotVerified, "cannot render target release: %v", err)
	}
	wls, err := parseWorkloads(workloads)
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	if runsDir == "" {
		runsDir, err = defaultRunsDir()
		if err != nil {
			return fail(ReasonInvalidConfig, err.Error())
		}
	}
	encoded, _ := json.Marshal(cfg)
	configDigest := fmt.Sprintf("sha256:%x", sha256.Sum256(encoded))
	store, run, err := toolkit.CreateHistoryRunKind("upgrade", runsDir, configDigest, toolkit.LockContentDigest(lock), "external-cluster cluster.reader/shadow", nil)
	if err != nil {
		return failf(ReasonInvalidConfig, "cannot open upgrade run: %v", err)
	}
	writer := "hnbctl-upgrade"
	active, err := store.AcquireWriter(writer, cfg.Spec.Timeouts.Readiness+cfg.Spec.Timeouts.Download+5*time.Minute)
	if err != nil {
		return failf(ReasonInvalidConfig, "cannot lease upgrade run: %v", err)
	}
	applier := toolkit.PlatformApplier{Kubeconfig: cfg.Spec.Cluster.KubeconfigFile, Namespace: cfg.Spec.Platform.Namespace,
		Manifests: rendered, Workloads: wls, Runner: toolkit.KubectlRunner{}, Timeout: cfg.Spec.Timeouts.Readiness}
	steps := toolkit.UpgradeSteps{Release: applier.Apply, Verify: applier.Verify}
	for _, s := range plan.Steps {
		if s.Kind == "backings" {
			// The rendered target release already carries the new data-plane
			// backing; apply it in the re-backing phase (idempotent) before the
			// release phase. Real dual-run/cutover against live backings is a
			// follow-up that needs the external/in-cluster endpoints.
			steps.Rebacking = applier.Apply
		}
	}
	if migrationsDir != "" {
		planes, perr := buildMigrationPlanes(ctx, cfg, migrationsDir)
		if perr != nil {
			return failf(ReasonPlatformNotVerified, "cannot resolve migration planes: %v", perr)
		}
		steps.Migrate = func(ctx context.Context) error {
			_, merr := toolkit.RunDatabaseMigrations(ctx, planes, nil)
			return merr
		}
	}
	result, err := toolkit.RunUpgrade(ctx, toolkit.UpgradeExecutionInput{
		Store: store, Writer: writer, Epoch: active.Epoch, Plan: plan,
		RequiresBackup: migrationsDir != "", BackupVerified: backupVerified, AllowIrreversible: allowIrreversible,
		Timeout: cfg.Spec.Timeouts.Readiness, Steps: steps,
	})
	if err != nil {
		return failf(ReasonPlatformNotVerified, "upgrade failed: %v (run %s)", err, run.RunID)
	}
	fmt.Fprintf(env.Stdout, "upgrade %s -> %s status=%s run=%s applied=%v\n", plan.From, plan.To, result.Status, run.RunID, result.Applied)
	return success("")
}

// runScale handles `hnbctl scale`: a scale plan with built-in guardrails. With
// --execute it applies the plan (deployment scale + PVC expansion) through the
// run store against the existing cluster.
func runScale(ctx context.Context, env *Env, args []string) Result {
	var configPath, profile, component, format, runsDir string
	var operatorReadyFlag string
	var controlNodeCount, replicas int
	var storage, current, currentStorage, cpu, memory, currentCPU, currentMemory stringSlice
	var execute bool
	fs := flag.NewFlagSet("scale", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&configPath, "f", "", "DeploymentConfig")
	fs.StringVar(&profile, "profile", "", "profile to scale (default: the config profile)")
	fs.StringVar(&component, "component", "", "component to scale (or postgresql|nats for the in-cluster data plane)")
	fs.IntVar(&replicas, "replicas", -1, "desired replica count for --component")
	fs.IntVar(&controlNodeCount, "control-node-count", 0, "observed schedulable control-plane domains (>=3 for in-cluster-ha; 0 = unknown)")
	fs.StringVar(&operatorReadyFlag, "operator-ready", "", "observed data-plane operator readiness: true|false (omit if unknown)")
	fs.Var(&storage, "storage", "storage expansion as name=Gi (repeatable)")
	fs.Var(&current, "current", "observed current replicas as name=N (repeatable; overrides the profile baseline)")
	fs.Var(&currentStorage, "current-storage", "observed current PVC Gi as name=Gi (repeatable)")
	fs.Var(&cpu, "cpu", "component cpu request as name=VALUE (repeatable)")
	fs.Var(&memory, "memory", "component memory request as name=VALUE (repeatable)")
	fs.Var(&currentCPU, "current-cpu", "observed current cpu request as name=VALUE (repeatable)")
	fs.Var(&currentMemory, "current-memory", "observed current memory request as name=VALUE (repeatable)")
	fs.BoolVar(&execute, "execute", false, "apply the plan (requires a certified profile and -f)")
	fs.StringVar(&runsDir, "runs-dir", "", "private scale history root (--execute)")
	fs.StringVar(&format, "format", "human", "human or json")
	if err := fs.Parse(args); err != nil {
		return fail(ReasonInvalidConfig, "invalid scale flags: "+err.Error())
	}
	if fs.NArg() != 0 {
		return fail(ReasonInvalidConfig, "unexpected scale arguments")
	}
	if format != "human" && format != "json" {
		return fail(ReasonInvalidConfig, "--format must be human or json")
	}
	if controlNodeCount < 0 {
		return fail(ReasonInvalidConfig, "--control-node-count must not be negative")
	}
	operatorReady, opRes := parseOperatorReady(operatorReadyFlag)
	if opRes.ExitCode != ExitSuccess {
		return opRes
	}
	if profile == "" {
		if configPath == "" {
			return fail(ReasonInvalidConfig, "provide -f or --profile to name the profile")
		}
		cfg, err := readConfig(configPath)
		if err != nil {
			return failf(ReasonInvalidConfig, "cannot read DeploymentConfig: %v", err)
		}
		profile = string(cfg.Spec.Profile)
	}
	shape, ok := toolkit.ProfileShape(toolkit.Profile(profile))
	if !ok {
		return fail(ReasonInvalidConfig, "unknown profile "+profile)
	}
	replicaReq := map[string]int{}
	if component != "" {
		if replicas < 0 {
			return fail(ReasonInvalidConfig, "--component requires --replicas")
		}
		replicaReq[component] = replicas
	}
	storageReq := map[string]int{}
	for _, item := range storage {
		name, value, ok := strings.Cut(item, "=")
		if !ok || name == "" {
			return fail(ReasonInvalidConfig, "--storage must be name=Gi")
		}
		gi, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || gi < 0 {
			return fail(ReasonInvalidConfig, "--storage value must be a non-negative integer Gi")
		}
		storageReq[name] = gi
	}
	if len(replicaReq) == 0 && len(storageReq) == 0 && len(cpu) == 0 && len(memory) == 0 {
		return fail(ReasonInvalidConfig, "scale requires --component/--replicas, --storage, --cpu or --memory")
	}
	resReq := map[string]toolkit.ResourceProfile{}
	setRes := func(name, c, m string) {
		r := resReq[name]
		if c != "" {
			r.CPU = c
		}
		if m != "" {
			r.Memory = m
		}
		resReq[name] = r
	}
	for _, item := range cpu {
		name, value, ok := strings.Cut(item, "=")
		if !ok || name == "" || value == "" {
			return fail(ReasonInvalidConfig, "--cpu must be name=VALUE")
		}
		setRes(name, value, "")
	}
	for _, item := range memory {
		name, value, ok := strings.Cut(item, "=")
		if !ok || name == "" || value == "" {
			return fail(ReasonInvalidConfig, "--memory must be name=VALUE")
		}
		setRes(name, "", value)
	}
	observedRes := map[string]toolkit.ResourceProfile{}
	observRes := func(items stringSlice, isCPU bool) Result {
		for _, item := range items {
			name, value, ok := strings.Cut(item, "=")
			if !ok || name == "" || value == "" {
				return fail(ReasonInvalidConfig, "observed resource must be name=VALUE")
			}
			r := observedRes[name]
			if isCPU {
				r.CPU = value
			} else {
				r.Memory = value
			}
			observedRes[name] = r
		}
		return success("")
	}
	if res := observRes(currentCPU, true); res.ExitCode != ExitSuccess {
		return res
	}
	if res := observRes(currentMemory, false); res.ExitCode != ExitSuccess {
		return res
	}
	observed := map[string]int{}
	for _, item := range current {
		name, value, ok := strings.Cut(item, "=")
		if !ok || name == "" {
			return fail(ReasonInvalidConfig, "--current must be name=N")
		}
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || n < 0 {
			return fail(ReasonInvalidConfig, "--current value must be a non-negative integer")
		}
		observed[name] = n
	}
	observedStorage := map[string]int{}
	for _, item := range currentStorage {
		name, value, ok := strings.Cut(item, "=")
		if !ok || name == "" {
			return fail(ReasonInvalidConfig, "--current-storage must be name=Gi")
		}
		n, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || n < 0 {
			return fail(ReasonInvalidConfig, "--current-storage value must be a non-negative integer Gi")
		}
		observedStorage[name] = n
	}
	plan, err := toolkit.PlanScale(toolkit.ScaleInput{Shape: shape, Replicas: replicaReq, StorageGi: storageReq, Resources: resReq, Observed: observed, ObservedStorage: observedStorage, ObservedResources: observedRes, ControlPlaneDomains: controlNodeCount, OperatorReady: operatorReady})
	if err != nil {
		return failf(ReasonInvalidConfig, "cannot plan scale: %v", err)
	}
	if format == "json" {
		if err := json.NewEncoder(env.Stdout).Encode(plan); err != nil {
			return fail(ReasonOutputFailed, "cannot write scale plan")
		}
	} else {
		fmt.Fprintf(env.Stdout, "scale %s authorized=%v\n", profile, plan.Authorized)
		for _, step := range plan.Steps {
			reversibility := "reversible"
			if !step.Reversible {
				reversibility = "irreversible"
			}
			fmt.Fprintf(env.Stdout, "  %s %s %s -> %s (%s)\n", step.Kind, step.Target, step.From, step.To, reversibility)
		}
		for _, reason := range plan.Blocked {
			fmt.Fprintf(env.Stdout, "  blocked: %s\n", reason)
		}
	}
	if execute {
		if !plan.Authorized {
			return fail(ReasonPlatformNotVerified, "scale execution refused: "+strings.Join(plan.Blocked, "; "))
		}
		return runScaleExecution(ctx, env, plan, configPath, runsDir)
	}
	return success("")
}

// runScaleExecution applies an authorized scale plan to the existing cluster.
// Component steps scale a Deployment and wait for rollout; storage steps expand
// a managed PVC (never shrink). It is only reached once PlanScale is authorized.
func runScaleExecution(ctx context.Context, env *Env, plan toolkit.ScalePlan, configPath, runsDir string) Result {
	if configPath == "" {
		return fail(ReasonInvalidConfig, "scale --execute requires -f to load the target configuration")
	}
	cfg, err := readConfig(configPath)
	if err != nil {
		return failf(ReasonInvalidConfig, "cannot read DeploymentConfig: %v", err)
	}
	if problems := cfg.Validate(toolkit.CurrentEvidence); len(problems) > 0 {
		return failf(ReasonInvalidConfig, "%s", strings.Join(problems, "; "))
	}
	if cfg.Spec.Mode != toolkit.ModeExisting {
		return fail(ReasonTargetActionMissing, "scale execution currently supports the existing-cluster route only")
	}
	if runsDir == "" {
		runsDir, err = defaultRunsDir()
		if err != nil {
			return fail(ReasonInvalidConfig, err.Error())
		}
	}
	store, run, err := toolkit.CreateHistoryRunKind("scale", runsDir, fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(configPath))), "", "external-cluster cluster.reader/shadow", nil)
	if err != nil {
		return failf(ReasonInvalidConfig, "cannot open scale run: %v", err)
	}
	writer := "hnbctl-scale"
	active, err := store.AcquireWriter(writer, cfg.Spec.Timeouts.Readiness+5*time.Minute)
	if err != nil {
		return failf(ReasonInvalidConfig, "cannot lease scale run: %v", err)
	}
	kubeconfig := cfg.Spec.Cluster.KubeconfigFile
	namespace := cfg.Spec.Platform.Namespace
	runner := toolkit.KubectlRunner{}
	steps := toolkit.ScaleSteps{
		Apply: func(ctx context.Context) error {
			for _, step := range plan.Steps {
				args, aerr := scaleStepArgs(step)
				if aerr != nil {
					return aerr
				}
				if args == nil {
					continue
				}
				if _, err := runner.Run(ctx, kubeconfig, nil, append([]string{"--namespace", namespace}, args...)...); err != nil {
					return fmt.Errorf("%s %s failed", step.Kind, step.Target)
				}
			}
			return nil
		},
		Verify: func(ctx context.Context) error {
			for _, step := range plan.Steps {
				if step.Kind != "components" {
					continue
				}
				args := scaleVerifyArgs(step.Target, cfg.Spec.Timeouts.Readiness.String())
				if _, err := runner.Run(ctx, kubeconfig, nil, append([]string{"--namespace", namespace}, args...)...); err != nil {
					return fmt.Errorf("readiness failed for %s", step.Target)
				}
			}
			return nil
		},
	}
	result, err := toolkit.RunScale(ctx, toolkit.ScaleExecutionInput{Store: store, Writer: writer, Epoch: active.Epoch, Plan: plan, Timeout: cfg.Spec.Timeouts.Readiness, Steps: steps})
	if err != nil {
		return failf(ReasonPlatformNotVerified, "scale failed: %v (run %s)", err, run.RunID)
	}
	fmt.Fprintf(env.Stdout, "scale status=%s run=%s applied=%v\n", result.Status, run.RunID, result.Applied)
	return success("")
}

// parseOperatorReady parses the tri-state --operator-ready flag: empty means the
// readiness was not observed (nil), otherwise true/false.
func parseOperatorReady(value string) (*bool, Result) {
	switch value {
	case "":
		return nil, success("")
	case "true":
		ready := true
		return &ready, success("")
	case "false":
		ready := false
		return &ready, success("")
	default:
		return nil, fail(ReasonInvalidConfig, "--operator-ready must be true or false")
	}
}

// scaleStepArgs maps a scale step to the kubectl args (without namespace) that
// apply it. Data-plane components patch the CloudNativePG Cluster instances or
// scale the JetStream StatefulSet; PostgreSQL storage patches the Cluster
// storage instead of a PVC.
func scaleStepArgs(step toolkit.UpgradeStep) ([]string, error) {
	switch step.Kind {
	case "components":
		switch step.Target {
		case "postgresql":
			return []string{"patch", "cluster.postgresql.cnpg.io/postgresql", "--type", "merge", "-p", `{"spec":{"instances":` + step.To + `}}`}, nil
		case "nats":
			return []string{"scale", "statefulset/nats", "--replicas", step.To}, nil
		default:
			return []string{"scale", "deployment/" + step.Target, "--replicas", step.To}, nil
		}
	case "storage":
		gi := strings.TrimSuffix(step.To, "Gi")
		if step.Target == "postgresql" {
			return []string{"patch", "cluster.postgresql.cnpg.io/postgresql", "--type", "merge", "-p", `{"spec":{"storage":{"size":"` + gi + `Gi"}}}`}, nil
		}
		return []string{"patch", "pvc", step.Target + "-data", "--type", "merge", "-p", `{"spec":{"resources":{"requests":{"storage":"` + gi + `Gi"}}}}`}, nil
	case "resources":
		parts := strings.SplitN(step.To, ",", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("resources for %s malformed", step.Target)
		}
		return []string{"set", "resources", "deployment/" + step.Target, "--requests=cpu=" + parts[0] + ",memory=" + parts[1]}, nil
	}
	return nil, nil
}

// scaleVerifyArgs maps a scale target to its readiness check args.
func scaleVerifyArgs(target, timeout string) []string {
	switch target {
	case "postgresql":
		return []string{"wait", "--for=condition=Ready", "cluster/postgresql", "--timeout", timeout}
	case "nats":
		return []string{"rollout", "status", "statefulset/nats", "--timeout", timeout}
	default:
		return []string{"rollout", "status", "deployment/" + target, "--timeout", timeout}
	}
}
