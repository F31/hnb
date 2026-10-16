package cli

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"
)

// parseFlags parses a -f config and optional -lock/-output/-runs-dir paths.
func parseFlags(args []string) (configPath, outputPath, lockPath, runsDir string, rest []string, err error) {
	fs := flag.NewFlagSet("hnbctl", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&configPath, "f", "", "DeploymentConfig path")
	fs.StringVar(&outputPath, "output", "", "output file path")
	fs.StringVar(&lockPath, "lock", "", "ReleaseLock path")
	fs.StringVar(&runsDir, "runs-dir", "", "install history root")
	if err := fs.Parse(args); err != nil {
		return "", "", "", "", nil, err
	}
	return configPath, outputPath, lockPath, runsDir, fs.Args(), nil
}

func runMatrix(_ context.Context, env *Env, _ []string) Result {
	ev := toolkit.CurrentEvidence
	fmt.Fprintf(env.Stdout, "evidence version: %s\n", ev.Version)
	fmt.Fprintln(env.Stdout, "evidence scope: provider/node/cluster method-level only; not full platform certification")
	fmt.Fprintf(env.Stdout, "certified OS: %s\n", strings.Join(ev.CertifiedOS, ", "))
	fmt.Fprintf(env.Stdout, "certified arch: %s\n", strings.Join(ev.CertifiedArch, ", "))
	for _, os := range ev.OSAdaptations {
		fmt.Fprintf(env.Stdout, "OS adaptation %s/%s kernel=%s: %s (route=%s distribution=%s; %s)\n", os.OS, os.Arch, os.Kernel, os.Status, os.Route, os.Distribution, os.Report)
	}
	for _, k := range ev.Kubernetes {
		fmt.Fprintf(env.Stdout, "kubernetes %s: %s (%s)\n", k.Version, k.Status, k.Report)
	}
	for _, c := range ev.CNI {
		fmt.Fprintf(env.Stdout, "cni %s: %s (%s)\n", c.Version, c.Status, c.Report)
	}
	for _, p := range ev.Profiles {
		fmt.Fprintf(env.Stdout, "profile %s: %s (%s)\n", p.Name, p.Status, strings.Join(p.Reasons, "; "))
	}
	fmt.Fprintln(env.Stdout, "route: existing is certified for Minimal full platform delivery; bootstrap and HA/offline remain blocked pending their own E2E")
	return success("")
}

func runInit(_ context.Context, env *Env, args []string) Result {
	var configPath, mode, profile, nodeOS, fromKubeconfig string
	var interactive bool
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&configPath, "f", "", "output path")
	fs.StringVar(&configPath, "file", "", "output path")
	fs.StringVar(&mode, "mode", "", "bootstrap or existing")
	fs.StringVar(&profile, "profile", "Minimal", "draft profile")
	fs.StringVar(&nodeOS, "os", "", "bootstrap OS recipe")
	fs.StringVar(&fromKubeconfig, "from-kubeconfig", "", "generate an existing-cluster draft from a kubeconfig (implies --mode existing)")
	fs.BoolVar(&interactive, "interactive", false, "prompt for draft choices")
	if err := fs.Parse(args); err != nil {
		return failf(ReasonInvalidConfig, "invalid init flags: %v", err)
	}
	if fs.NArg() != 0 {
		return fail(ReasonInvalidConfig, "unexpected init arguments")
	}
	if fromKubeconfig != "" {
		if interactive {
			return fail(ReasonInvalidConfig, "--from-kubeconfig cannot be combined with --interactive")
		}
		if mode != "" && mode != "existing" {
			return fail(ReasonInvalidConfig, "--from-kubeconfig implies --mode existing")
		}
		mode = "existing"
	}
	if interactive {
		if env.Stdin == nil {
			return fail(ReasonInvalidConfig, "interactive init requires input")
		}
		scanner := bufio.NewScanner(env.Stdin)
		for _, prompt := range []struct {
			label string
			value *string
		}{
			{"mode (bootstrap|existing)", &mode},
			{"profile", &profile},
			{"bootstrap OS (see hnbctl os; blank for existing)", &nodeOS},
			{"output file", &configPath},
		} {
			fmt.Fprintf(env.Stdout, "%s [%s]: ", prompt.label, *prompt.value)
			if !scanner.Scan() {
				return fail(ReasonInvalidConfig, "interactive input ended before draft choices were complete")
			}
			if value := strings.TrimSpace(scanner.Text()); value != "" {
				*prompt.value = value
			}
		}
	}
	if configPath == "" {
		return fail(ReasonInvalidConfig, "-f is required (output DeploymentConfig path)")
	}
	if mode == "" {
		return fail(ReasonInvalidConfig, "--mode bootstrap|existing is required")
	}
	if mode != "bootstrap" && mode != "existing" {
		return fail(ReasonInvalidConfig, "--mode must be bootstrap or existing")
	}
	if mode == "existing" && nodeOS != "" {
		return fail(ReasonInvalidConfig, "--os is only valid for bootstrap drafts")
	}
	if mode == "bootstrap" && nodeOS == "" {
		nodeOS = "ubuntu-24.04"
	}
	if mode == "bootstrap" {
		if _, ok := toolkit.LookupNodeOSAdapter(nodeOS); !ok {
			return fail(ReasonInvalidConfig, "unsupported bootstrap OS draft; see hnbctl os")
		}
	}
	tmpl, err := templateConfig(mode, profile)
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	if adapter, ok := toolkit.LookupNodeOSAdapter(nodeOS); ok {
		tmpl = strings.Replace(tmpl, "user: ubuntu", "user: "+adapter.DefaultSSHUser, 1)
		tmpl = "# Node OS adapter: " + adapter.OS + "; select a matching release lock (draft only).\n" + tmpl
	}
	endpoint := ""
	if fromKubeconfig != "" {
		absKubeconfig, err := filepath.Abs(fromKubeconfig)
		if err != nil {
			return fail(ReasonInvalidConfig, "cannot resolve --from-kubeconfig path")
		}
		endpoint, err = toolkit.ReadKubeconfigEndpoint(absKubeconfig)
		if err != nil {
			return failf(ReasonInvalidConfig, "cannot use --from-kubeconfig (%v)", err)
		}
		tmpl = strings.Replace(tmpl, `kubeconfigFile: ""`, "kubeconfigFile: "+tmplYAMLString(absKubeconfig), 1)
	}
	file, err := os.OpenFile(configPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return failf(ReasonInvalidConfig, "write %s: %v", configPath, err)
	}
	_, writeErr := io.WriteString(file, tmpl)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		return failf(ReasonInvalidConfig, "write draft: write=%v close=%v", writeErr, closeErr)
	}
	if endpoint != "" {
		fmt.Fprintf(env.Stdout, "created %s (mode=existing profile=%s). Observed API server %s. Still required: real entry (publicBaseURL), TLS Secret, verified StorageClass and release lock. A draft is not validation.\n", configPath, profile, endpoint)
		return success("")
	}
	fmt.Fprintf(env.Stdout, "created %s (mode=%s profile=%s). Fill required values (entry, storage, credentials, release lock) before install.\n", configPath, mode, profile)
	return success("")
}

// tmplYAMLString quotes a path for safe inclusion in a generated YAML template.
func tmplYAMLString(value string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(value, `\`, `\\`), `"`, `\"`) + `"`
}

func templateConfig(mode, profile string) (string, error) {
	if profile != "Minimal" {
		return "", fmt.Errorf("profile %s is not certified; only Minimal configuration drafts are available (platform installation remains blocked)", profile)
	}
	if mode == "existing" {
		return `apiVersion: deployment.hnb.io/v1alpha1
kind: DeploymentConfig
metadata:
  name: existing-minimal
spec:
  mode: existing
  profile: Minimal
  releaseLock: ./release.lock.yaml
  cluster:
    strategy: existing
    kubeconfigFile: ""            # required: path to the external cluster kubeconfig
  platform:
    namespace: hnb-system
    publicBaseURL: ""             # required: real entry; no default fake domain
    tls:
      existingSecret: ""          # required: TLS Secret in the platform namespace
    storage:
      className: ""               # required: a StorageClass verified on the target
    infrastructure:
      postgresql:
        mode: bundled             # or external + secretRef
      nats:
        mode: bundled             # or external + secretRef
      # Each owned plane gets its own database, role and credential. Sharing one
      # across planes would let one plane reach another's tables.
      planes:
        - name: platform
          database: hnb_platform
          role: hnb_platform
          postgresqlSecretRef: platform-postgresql
          natsSecretRef: platform-nats
        - name: market
          database: hnb_market
          role: hnb_market
          postgresqlSecretRef: market-postgresql
          natsSecretRef: market-nats
        - name: artifact
          database: hnb_artifact
          role: hnb_artifact
          postgresqlSecretRef: artifact-postgresql
          natsSecretRef: artifact-nats
  artifacts:
    mode: online                  # online | mirror | offline
    bundlePath: ""
  timeouts:
    download: 15m
    readiness: 10m
`, nil
	}
	return `apiVersion: deployment.hnb.io/v1alpha1
kind: DeploymentConfig
metadata:
  name: bootstrap-minimal
spec:
  mode: bootstrap
  profile: Minimal
  releaseLock: ./release.lock.yaml
  cluster:
    strategy: provision
    distribution: kubeadm
    kubernetesVersion: 1.36.5
    apiEndpoint: ""               # required: control-plane endpoint IP:6443
    nodes:
      - name: cp-1
        roles: [control-plane]
        sshAddress: ""            # required: reachable IP
        internalAddress: ""       # required: node IP used by the cluster
        user: ubuntu              # non-root requires passwordless sudo
        sshKeyFile: ""            # required: path to an authorized SSH key
    network:
      plugin: cilium
      version: 1.20.1
      podCIDR: 10.244.0.0/16
      serviceCIDR: 10.96.0.0/12
      routingMode: native
  platform:
    namespace: hnb-system
    publicBaseURL: ""             # required: real entry; no default fake domain
    tls:
      existingSecret: ""
    storage:
      className: ""
    infrastructure:
      postgresql:
        mode: bundled
      nats:
        mode: bundled
      # Each owned plane gets its own database, role and credential. Sharing one
      # across planes would let one plane reach another's tables.
      planes:
        - name: platform
          database: hnb_platform
          role: hnb_platform
          postgresqlSecretRef: platform-postgresql
          natsSecretRef: platform-nats
        - name: market
          database: hnb_market
          role: hnb_market
          postgresqlSecretRef: market-postgresql
          natsSecretRef: market-nats
        - name: artifact
          database: hnb_artifact
          role: hnb_artifact
          postgresqlSecretRef: artifact-postgresql
          natsSecretRef: artifact-nats
  artifacts:
    mode: online
    bundlePath: ""
  timeouts:
    download: 15m
    readiness: 10m
`, nil
}

func runValidate(_ context.Context, env *Env, args []string) Result {
	configPath, outputPath, lockPath, _, rest, err := parseFlags(args)
	if err != nil {
		return failf(ReasonInvalidConfig, "invalid flags: %v", err)
	}
	if configPath == "" {
		return fail(ReasonInvalidConfig, "-f is required")
	}
	if len(rest) > 0 || outputPath != "" {
		return fail(ReasonInvalidConfig, "validate does not accept positional arguments or --output")
	}
	cfg, err := readConfig(configPath)
	if err != nil {
		return failf(ReasonInvalidConfig, "read config: %v", err)
	}
	problems := cfg.Validate(toolkit.CurrentEvidence)
	if len(problems) > 0 {
		for _, p := range problems {
			fmt.Fprintln(env.Stderr, "config:", p)
		}
		return fail(ReasonInvalidConfig, "DeploymentConfig is invalid")
	}
	if problems := cfg.ValidateCredentialFiles(); len(problems) > 0 {
		return fail(ReasonInvalidConfig, strings.Join(problems, "; "))
	}
	if _, _, err := readValidatedLock(cfg, lockPath); err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	if gate := toolkit.EvidenceGate(cfg, toolkit.CurrentEvidence); len(gate) > 0 {
		return fail(ReasonProfileBlocked, "configuration structure checked; delivery blocked: "+strings.Join(gate, "; "))
	}
	fmt.Fprintln(env.Stdout, "ReleaseLock structure and declared compatibility checked; artifact contents are not verified by validate")
	fmt.Fprintln(env.Stdout, "DeploymentConfig is structurally valid")
	return success("")
}

// notImplemented returns the explicit non-success outcome for commands whose
// machinery is not yet shipped, per DPLY-008. It never prints a fake success.
func notImplemented(name, next string) Result {
	return failf(ReasonTargetActionMissing, "%s is not implemented yet; %s", name, next)
}

func runPreflight(ctx context.Context, env *Env, args []string) Result {
	var configPath, outputPath, lockPath, format string
	fs := flag.NewFlagSet("preflight", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&configPath, "f", "", "DeploymentConfig path")
	fs.StringVar(&outputPath, "output", "", "private JSON report file")
	fs.StringVar(&lockPath, "lock", "", "ReleaseLock override")
	fs.StringVar(&format, "format", "human", "stdout format: human or json")
	var sshAuth sshAuthFlags
	sshAuth.register(fs)
	if err := fs.Parse(args); err != nil {
		return failf(ReasonInvalidConfig, "invalid flags: %v", err)
	}
	if format != "human" && format != "json" {
		return fail(ReasonInvalidConfig, "--format must be human or json")
	}
	if configPath == "" {
		return fail(ReasonInvalidConfig, "-f is required")
	}
	if fs.NArg() != 0 {
		return fail(ReasonInvalidConfig, "unexpected preflight arguments")
	}
	cfg, err := readConfig(configPath)
	if err != nil {
		return failf(ReasonInvalidConfig, "%v", err)
	}
	runner, err := sshAuth.runner(env, cfg)
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	if problems := cfg.Validate(toolkit.CurrentEvidence); len(problems) > 0 {
		return fail(ReasonInvalidConfig, "DeploymentConfig is invalid: "+strings.Join(problems, "; "))
	}
	if problems := cfg.ValidateCredentialFiles(); len(problems) > 0 {
		return fail(ReasonInvalidConfig, strings.Join(problems, "; "))
	}
	lock, lockData, err := readValidatedLock(cfg, lockPath)
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	configData, err := json.Marshal(cfg)
	if err != nil {
		return fail(ReasonInvalidConfig, "cannot compute normalized configuration identity")
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return fail(ReasonInvalidConfig, "cannot generate preflight identity")
	}
	report := toolkit.PreflightReport{SchemaVersion: toolkit.PreflightReportSchema, RunID: hex.EncodeToString(id), ConfigDigest: fmt.Sprintf("sha256:%x", sha256.Sum256(configData)), LockDigest: fmt.Sprintf("sha256:%x", sha256.Sum256(lockData))}
	if cfg.Spec.Cluster.Strategy == toolkit.StrategyProvision {
		report.Checks = append(report.Checks, toolkit.ProbeLockedNodes(ctx, cfg, lock, runner)...)
		report.Checks = append(report.Checks, toolkit.ProbeNodeAPIPaths(ctx, cfg, runner)...)
	} else {
		report.Checks = append(report.Checks, toolkit.ProbeExistingKubeconfig(ctx, cfg.Spec.Cluster.KubeconfigFile)...)
	}
	httpProbe := toolkit.NewHTTPProbe(nil)
	if cfg.Spec.Artifacts.Mode != toolkit.ArtifactsOffline {
		probeCtx, cancel := context.WithTimeout(ctx, cfg.Spec.Timeouts.Download)
		report.Checks = append(report.Checks, httpProbe.ProbeEntry(probeCtx, cfg.Spec.Platform.PublicBaseURL))
		report.Checks = append(report.Checks, httpProbe.ProbeLockedArtifacts(probeCtx, lock)...)
		cancel()
	} else {
		report.Checks = append(report.Checks, toolkit.PreflightCheck{CheckID: "artifacts.offline-closure", Scope: "artifacts", ObservedAt: time.Now().UTC(), Status: toolkit.StatusBlocked, Expected: "verified local dependency closure without public requests", Actual: "offline closure probe not implemented; no public artifact or entry requests made", ReasonCode: "OFFLINE_CLOSURE_UNVERIFIED", Remediation: "Complete bundle closure validation before offline installation."})
	}
	for _, reason := range toolkit.EvidenceGate(cfg, toolkit.CurrentEvidence) {
		category, _, _ := strings.Cut(reason, " ")
		report.Checks = append(report.Checks, toolkit.PreflightCheck{CheckID: "delivery.evidence." + category, Scope: "platform", ObservedAt: time.Now().UTC(), Status: toolkit.StatusBlocked, Expected: "independent profile/route/offline delivery evidence", Actual: reason, ReasonCode: "DELIVERY_EVIDENCE_MISSING", Remediation: "Complete the required acceptance scope before installing."})
	}
	report.Checks = append(report.Checks, toolkit.PreflightCheck{CheckID: "preflight.completeness", Scope: "platform", ObservedAt: time.Now().UTC(), Status: toolkit.StatusBlocked, Expected: "complete recipe, network, API, storage, TLS and artifact probes", Actual: "node prerequisites, node API TCP, installer HTTPS/content and existing kubeconfig/API TLS probes implemented; provider/Agent, DNS/Pod routing, existing storage and node-source package/image paths remain unverified", ReasonCode: "PREFLIGHT_CHECK_SET_INCOMPLETE", Remediation: "Complete task 2.5 and resolved recipes in 3.1/3.2; this partial report cannot authorize install."})
	report.GeneratedAt = time.Now().UTC()
	if err := report.Validate(); err != nil {
		return fail(ReasonOutputFailed, "invalid preflight report: "+err.Error())
	}
	if outputPath != "" {
		file, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return failf(ReasonInvalidConfig, "create report: %v", err)
		}
		writeErr := toolkit.WritePreflightReport(file, &report, "json")
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			return fail(ReasonOutputFailed, "cannot write preflight report")
		}
	}
	if err := toolkit.WritePreflightReport(env.Stdout, &report, format); err != nil {
		return fail(ReasonOutputFailed, "cannot write preflight stdout")
	}
	return fail(ReasonPreflightBlocked, "preflight blocked; partial checks and missing delivery evidence cannot authorize installation")
}

func readValidatedLock(cfg *toolkit.DeploymentConfig, path string) (*toolkit.ReleaseLock, []byte, error) {
	if path == "" {
		path = cfg.Spec.ReleaseLock
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read lock: %w", err)
	}
	lock, err := toolkit.DecodeReleaseLock(data)
	if err != nil {
		return nil, nil, fmt.Errorf("decode lock: %w", err)
	}
	if problems := lock.Validate(); len(problems) > 0 {
		return nil, nil, fmt.Errorf("ReleaseLock is invalid: %s", strings.Join(problems, "; "))
	}
	if problems := lock.ValidateCompatibility(cfg, toolkit.CurrentEvidence); len(problems) > 0 {
		return nil, nil, fmt.Errorf("ReleaseLock is invalid: %s", strings.Join(problems, "; "))
	}
	return lock, data, nil
}

func runInstall(ctx context.Context, env *Env, args []string) Result {
	opts, err := parseInstallOptions(args)
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	cfg, err := opts.configuration(env)
	if err != nil {
		return failf(ReasonInvalidConfig, "cannot read DeploymentConfig: %v", err)
	}
	if problems := cfg.Validate(toolkit.CurrentEvidence); len(problems) > 0 {
		return failf(ReasonInvalidConfig, "%s", strings.Join(problems, "; "))
	}
	if cfg.Spec.Mode != toolkit.ModeExisting && !opts.quick && !opts.plan && !opts.execute {
		return notImplemented("install", "bootstrap Day-0 remains blocked; the dedicated blank management-base bootstrap adapter and durable receipt are 4.4/4.5")
	}
	// Existing-cluster route: derive the strict dependency plan + ownership
	// manifest. This performs no cluster/node mutation and no node-init action.
	lock, lockData, err := readValidatedLock(cfg, opts.lock)
	if err != nil {
		return failf(ReasonInvalidConfig, "%v", err)
	}
	if opts.execute && cfg.Spec.Mode == toolkit.ModeBootstrap {
		return runBootstrapExecution(ctx, env, cfg, lock, opts)
	}
	var ownership *toolkit.OwnershipManifest
	var plan any
	routing := "bootstrap"
	var claimed []string
	var reasons []string
	if cfg.Spec.Mode == toolkit.ModeExisting {
		var ownProblems []string
		ownership, ownProblems = cfg.Ownership()
		if len(ownProblems) > 0 {
			return failf(ReasonInvalidConfig, "ownership boundary invalid: %s", strings.Join(ownProblems, "; "))
		}
		if !ownership.FitsBoundary || !ownership.NoNodeInitAction {
			return fail(ReasonTargetActionMissing, "existing-cluster install would cross the ownership boundary (external CNI/owner must be preserved and no node-init action may be created)")
		}
		existing, problems := cfg.PlanExistingDependencies(lock)
		if len(problems) > 0 {
			return fail(ReasonInvalidConfig, "existing install plan invalid: "+strings.Join(problems, "; "))
		}
		plan, routing, claimed = existing, existing.RoutingMode, ownership.Claimed
		reasons = append(reasons,
			"NO_NODE_INIT_ACTION: the external cluster/CNI lifecycle owner is preserved; no bootstrap/join/reset action is generated")
		if !opts.execute {
			reasons = append(reasons, "PLANNING_ONLY: existing-route Minimal delivery is certified, but this invocation records a plan and does not apply; use install --execute --chart-package ... --bindings ... to execute")
		}
	} else {
		plan = map[string]any{"route": routing, "nodes": cfg.Spec.Cluster.Nodes, "namespace": cfg.Spec.Platform.Namespace,
			"profile": cfg.Spec.Profile, "executionAuthorized": false}
		reasons = append(reasons,
			"INSTALL_EXECUTOR_UNAVAILABLE: blank-host installation requires the gated bootstrap execution path")
		reasons = append(reasons, toolkit.EvidenceGate(cfg, toolkit.CurrentEvidence)...)
	}
	runsDir := opts.runsDir
	if runsDir == "" {
		var err error
		runsDir, err = defaultRunsDir()
		if err != nil {
			return fail(ReasonInvalidConfig, err.Error())
		}
	}
	encoded, err := json.Marshal(cfg)
	if err != nil {
		return fail(ReasonInvalidConfig, "cannot compute configuration identity")
	}
	configDigest := fmt.Sprintf("sha256:%x", sha256.Sum256(encoded))
	store, run, err := toolkit.CreateHistoryRun(runsDir, configDigest, toolkit.LockContentDigest(lock), routing, claimed)
	if err != nil {
		return failf(ReasonInvalidConfig, "cannot open install run: %v", err)
	}
	writer := "hnbctl-install"
	active, err := store.AcquireWriter(writer, cfg.Spec.Timeouts.Download+cfg.Spec.Timeouts.Readiness+5*time.Minute)
	if err != nil {
		return failf(ReasonInvalidConfig, "cannot lease install run: %v", err)
	}
	if _, err := store.BeginPhase(writer, active.Epoch, toolkit.PhasePlanning, nil); err != nil {
		return failf(ReasonInvalidConfig, "cannot record planning: %v", err)
	}
	if err := store.SaveInstallSnapshot(cfg, lockData); err != nil {
		_, _ = store.FailPhase(writer, active.Epoch, toolkit.PhasePlanning, "INSTALL_SNAPSHOT_FAILED")
		_, _ = store.Finalize(writer, active.Epoch, toolkit.PhaseFailedState, nil)
		return fail(ReasonOutputFailed, err.Error())
	}
	if _, err := store.CompletePhase(writer, active.Epoch, toolkit.PhasePlanning, nil, []string{"dependency plan and normalized install snapshots saved"}); err != nil {
		return failf(ReasonInvalidConfig, "cannot record planning: %v", err)
	}
	result := fail(ReasonPlatformNotVerified, fmt.Sprintf("install plan computed (planning only; installation=%s); execution is unavailable", run.RunID))
	if opts.quick && !opts.plan {
		if _, err := store.BeginPhase(writer, active.Epoch, toolkit.PhasePreflight, nil); err != nil {
			return fail(ReasonOutputFailed, "cannot record preflight start")
		}
		probeEnv := *env
		probeEnv.Stdout = io.Discard
		probeArgs := []string{"-f", filepath.Join(store.Dir, "deployment.yaml"), "--lock", filepath.Join(store.Dir, "release.lock.yaml"), "--output", filepath.Join(store.Dir, "preflight.json")}
		if opts.auth.ask {
			probeArgs = append(probeArgs, "--ask-pass")
		}
		if opts.auth.passwordFile != "" {
			probeArgs = append(probeArgs, "--ssh-password-file", opts.auth.passwordFile)
		}
		if opts.auth.askSudo {
			probeArgs = append(probeArgs, "--ask-sudo-pass")
		}
		if opts.auth.sudoFile != "" {
			probeArgs = append(probeArgs, "--sudo-password-file", opts.auth.sudoFile)
		}
		probe := runPreflight(ctx, &probeEnv, probeArgs)
		if probe.ExitCode == ExitSuccess {
			if _, err := store.CompletePhase(writer, active.Epoch, toolkit.PhasePreflight, nil, []string{"preflight.json"}); err != nil {
				return fail(ReasonOutputFailed, "cannot record preflight outcome")
			}
		} else {
			if _, err := store.FailPhase(writer, active.Epoch, toolkit.PhasePreflight, string(probe.Reason)); err != nil {
				return fail(ReasonOutputFailed, "cannot record preflight outcome")
			}
			reasons = append(reasons, string(probe.Reason)+": "+probe.Message)
			result = probe
		}
	}
	executed := false
	if opts.execute {
		// The execution path is reached only with an explicit request. It always
		// enforces the delivery evidence gate first; no node or cluster is
		// contacted while the gate refuses, and nothing is mutated.
		gate := toolkit.EvidenceGate(cfg, toolkit.CurrentEvidence)
		if len(gate) > 0 {
			result = fail(ReasonPlatformNotVerified, "install execution refused: "+strings.Join(gate, "; "))
		} else if cfg.Spec.Mode != toolkit.ModeExisting {
			result = fail(ReasonTargetActionMissing, "install execution refused: the platform applier currently supports the existing-cluster route; bootstrap additionally requires the wired management adapter")
		} else if opts.chartPackage == "" || opts.bindings == "" {
			result = fail(ReasonInvalidConfig, "install execution requires --chart-package and --bindings to render the platform release")
		} else if applierErr := runInstallExecution(ctx, env, store, writer, active.Epoch, cfg, lock, opts); applierErr != nil {
			result = failf(ReasonPlatformNotVerified, "%v", applierErr)
		} else {
			result = success("")
			executed = true
		}
	}
	doc := map[string]any{
		"schemaVersion":       "install-plan/v1",
		"installationId":      run.RunID,
		"run":                 run.RunID,
		"runDir":              store.Dir,
		"configPath":          filepath.Join(store.Dir, "deployment.yaml"),
		"executionAuthorized": executed,
		"ownership":           ownership,
		"plan":                plan,
		"reasons":             reasons,
	}
	finalState := toolkit.PhaseBlockedState
	var completed []toolkit.PhaseName
	if executed {
		finalState = toolkit.PhaseSucceededState
		completed = []toolkit.PhaseName{toolkit.PhasePlatformRelease, toolkit.PhasePlatformVerified}
		if opts.migrationsDir != "" {
			completed = append(completed, toolkit.PhaseDatabaseMigrate)
		}
	}
	if _, err := store.Finalize(writer, active.Epoch, finalState, completed); err != nil {
		return failf(ReasonInvalidConfig, "cannot record planning outcome: %v", err)
	}
	encodedPlan, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fail(ReasonInvalidConfig, "cannot encode existing-cluster plan")
	}
	if opts.output != "" {
		file, err := os.OpenFile(opts.output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return failf(ReasonInvalidConfig, "create plan: %v", err)
		}
		writeErr := json.NewEncoder(file).Encode(doc)
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			return fail(ReasonOutputFailed, "cannot write existing-cluster plan")
		}
	} else if opts.format == "json" {
		if _, err := fmt.Fprintln(env.Stdout, string(encodedPlan)); err != nil {
			return fail(ReasonOutputFailed, "cannot write install plan")
		}
	} else {
		state := "blocked"
		if executed {
			state = "succeeded"
		}
		fmt.Fprintf(env.Stdout, "installation=%s state=%s executionAuthorized=%t\nroute=%s profile=%s namespace=%s entry=%s\nconfiguration=%s\n", run.RunID, state, executed, routing, cfg.Spec.Profile, cfg.Spec.Platform.Namespace, cfg.Spec.Platform.PublicBaseURL, filepath.Join(store.Dir, "deployment.yaml"))
		if cfg.Spec.Mode == toolkit.ModeBootstrap {
			fmt.Fprintf(env.Stdout, "host=%s user=%s internalAddress=%s\n", cfg.Spec.Cluster.Nodes[0].SSHAddress, cfg.Spec.Cluster.Nodes[0].User, cfg.Spec.Cluster.Nodes[0].InternalAddress)
		}
		if !executed {
			for _, reason := range reasons {
				fmt.Fprintf(env.Stdout, "  %s\n", reason)
			}
		}
	}
	if opts.plan || executed {
		return success("")
	}
	return result
}

func readConfig(path string) (*toolkit.DeploymentConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg, err := toolkit.DecodeConfig(data)
	if err != nil {
		return nil, err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	if err := cfg.Normalize(filepath.Dir(absPath), homeDir); err != nil {
		return nil, err
	}
	return cfg, nil
}
