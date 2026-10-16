package cli

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"
	"golang.org/x/term"
)

type installOptions struct {
	config, output, lock, runsDir                                                         string
	host, internal, user, key, kubeconfig, profile, domain, namespace, name, tls, storage string
	format                                                                                string
	plan, yes, execute                                                                    bool
	chartPackage, bindings                                                                string
	workloads                                                                             stringSlice
	imageRepository, sandboxImage, cniImageRepository, chartRepository                    string
	migrationsDir                                                                         string
	auth                                                                                  sshAuthFlags
	quick                                                                                 bool
}

// stringSlice collects a repeatable flag value.
type stringSlice []string

func (s *stringSlice) String() string { return strings.Join(*s, ",") }
func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func parseInstallOptions(args []string) (installOptions, error) {
	var o installOptions
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.config, "f", "", "DeploymentConfig (exclusive with shortcut configuration flags)")
	fs.StringVar(&o.output, "output", "", "plan output file")
	fs.StringVar(&o.lock, "lock", "", "ReleaseLock path (shortcut default: ./release.lock.yaml)")
	fs.StringVar(&o.runsDir, "runs-dir", "", "private installation history root")
	fs.StringVar(&o.host, "host", "", "single blank host IPv4 address")
	fs.StringVar(&o.internal, "internal-address", "", "node address used by Kubernetes (default: host)")
	fs.StringVar(&o.user, "user", "ubuntu", "SSH user")
	fs.StringVar(&o.key, "ssh-key", "", "authorized SSH private key")
	fs.StringVar(&o.kubeconfig, "kubeconfig", "", "existing Kubernetes cluster kubeconfig")
	fs.StringVar(&o.profile, "profile", "Minimal", "complete platform profile")
	fs.StringVar(&o.domain, "domain", "", "public platform hostname (HTTPS)")
	fs.StringVar(&o.namespace, "namespace", "hnb-system", "platform namespace")
	fs.StringVar(&o.name, "name", "hnb", "installation configuration name")
	fs.StringVar(&o.tls, "tls-secret", "", "existing TLS Secret in the platform namespace")
	fs.StringVar(&o.storage, "storage-class", "", "verified target StorageClass")
	fs.StringVar(&o.format, "format", "", "human or json (shortcut default: human; -f default: json)")
	fs.BoolVar(&o.plan, "plan", false, "only generate a plan/snapshots; no remote probes or mutations")
	fs.BoolVar(&o.execute, "execute", false, "attempt the install execution path; still refuses unless delivery evidence is certified and an applier exists")
	fs.StringVar(&o.chartPackage, "chart-package", "", "verified chart package to render for the platform applier (--execute)")
	fs.StringVar(&o.bindings, "bindings", "", "render bindings file (Secret references) for the platform applier (--execute)")
	fs.Var(&o.workloads, "workload", "workload to wait for rollout as Kind/name (repeatable; --execute)")
	fs.StringVar(&o.imageRepository, "image-repository", "", "pinned node/kubernetes image repository (bootstrap --execute)")
	fs.StringVar(&o.sandboxImage, "sandbox-image", "", "pinned sandbox (pause) image reference (bootstrap --execute)")
	fs.StringVar(&o.cniImageRepository, "cni-image-repository", "", "pinned CNI image repository (bootstrap --execute)")
	fs.StringVar(&o.chartRepository, "chart-repository", "", "locked Helm chart repository URL (bootstrap --execute)")
	fs.StringVar(&o.migrationsDir, "migrations-dir", "", "prepared migration material root (<dir>/<owner>/*.sql); enables the gated migration stage (existing --execute)")
	fs.BoolVar(&o.yes, "yes", false, "confirm changes when an executable route becomes available; does not bypass gates")
	o.auth.register(fs)
	if err := fs.Parse(args); err != nil {
		return o, fmt.Errorf("invalid install flags: %v", err)
	}
	if fs.NArg() != 0 {
		return o, fmt.Errorf("unexpected install arguments")
	}
	shortcut := map[string]bool{"host": true, "internal-address": true, "user": true, "ssh-key": true, "kubeconfig": true, "profile": true, "domain": true, "namespace": true, "name": true, "tls-secret": true, "storage-class": true}
	var mixed, hostOnly bool
	fs.Visit(func(f *flag.Flag) {
		if shortcut[f.Name] {
			mixed = true
		}
		if f.Name == "user" || f.Name == "internal-address" {
			hostOnly = true
		}
	})
	if o.config != "" && mixed {
		return o, fmt.Errorf("choose -f or shortcut configuration flags, not both")
	}
	o.quick = o.config == ""
	if o.plan && o.execute {
		return o, fmt.Errorf("--plan and --execute are mutually exclusive")
	}
	if o.quick && ((o.host == "") == (o.kubeconfig == "")) {
		return o, fmt.Errorf("provide exactly one of --host or --kubeconfig (or use -f)")
	}
	if o.auth.ask && o.auth.passwordFile != "" || o.auth.askSudo && o.auth.sudoFile != "" {
		return o, fmt.Errorf("choose a password prompt or file, not both")
	}
	if o.quick && o.key != "" && (o.auth.ask || o.auth.passwordFile != "") {
		return o, fmt.Errorf("choose --ssh-key or password authentication")
	}
	if o.kubeconfig != "" && (o.key != "" || o.auth.ask || o.auth.passwordFile != "" || o.auth.askSudo || o.auth.sudoFile != "") {
		return o, fmt.Errorf("existing clusters use kubeconfig, not SSH authentication flags")
	}
	if o.kubeconfig != "" && hostOnly {
		return o, fmt.Errorf("--user and --internal-address apply only to --host")
	}
	if o.format == "" {
		if o.quick {
			o.format = "human"
		} else {
			o.format = "json"
		}
	}
	if o.format != "human" && o.format != "json" {
		return o, fmt.Errorf("--format must be human or json")
	}
	return o, nil
}

func (o *installOptions) configuration(env *Env) (*toolkit.DeploymentConfig, error) {
	if !o.quick {
		cfg, err := readConfig(o.config)
		if err == nil && cfg.Spec.Cluster.Strategy == toolkit.StrategyExisting && (o.auth.ask || o.auth.passwordFile != "" || o.auth.askSudo || o.auth.sudoFile != "") {
			return nil, fmt.Errorf("existing clusters use kubeconfig, not SSH authentication flags")
		}
		if err == nil && (o.auth.ask || o.auth.passwordFile != "") {
			for i := range cfg.Spec.Cluster.Nodes {
				cfg.Spec.Cluster.Nodes[i].SSHAuth = "password"
			}
		}
		return cfg, err
	}
	// Only ask for target-specific values when there is a real interactive terminal.
	input, terminal := env.Stdin.(*os.File)
	terminal = terminal && term.IsTerminal(int(input.Fd()))
	var reader *bufio.Reader
	if terminal {
		reader = bufio.NewReader(input)
	}
	for _, item := range []struct {
		name  string
		value *string
	}{{"domain", &o.domain}, {"tls-secret", &o.tls}, {"storage-class", &o.storage}} {
		if *item.value != "" {
			continue
		}
		if !terminal {
			return nil, fmt.Errorf("--%s is required without an interactive terminal", item.name)
		}
		fmt.Fprintf(env.Stderr, "%s: ", item.name)
		line, err := reader.ReadString('\n')
		if err != nil || strings.TrimSpace(line) == "" {
			return nil, fmt.Errorf("--%s is required", item.name)
		}
		*item.value = strings.TrimSpace(line)
	}
	u, err := url.Parse("https://" + o.domain)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("--domain must be a hostname, optionally with a port, without scheme/path/credentials")
	}
	mode := "existing"
	if o.host != "" {
		mode = "bootstrap"
	}
	draft, err := templateConfig(mode, o.profile)
	if err != nil {
		return nil, err
	}
	cfg, err := toolkit.DecodeConfig([]byte(draft))
	if err != nil {
		return nil, err
	}
	cfg.Metadata.Name = o.name
	cfg.Spec.Platform.Namespace = o.namespace
	cfg.Spec.Platform.PublicBaseURL = u.String()
	cfg.Spec.Platform.TLS.ExistingSecret = o.tls
	cfg.Spec.Platform.Storage.ClassName = o.storage
	if mode == "existing" {
		cfg.Spec.Cluster.KubeconfigFile = o.kubeconfig
	} else {
		if o.internal == "" {
			o.internal = o.host
		}
		cfg.Spec.Cluster.APIEndpoint = net.JoinHostPort(o.internal, "6443")
		n := &cfg.Spec.Cluster.Nodes[0]
		n.SSHAddress, n.InternalAddress, n.User, n.SSHKeyFile = o.host, o.internal, o.user, o.key
		if o.auth.ask || o.auth.passwordFile != "" {
			n.SSHAuth = "password"
		} else if o.key == "" {
			if !terminal {
				return nil, fmt.Errorf("--host requires --ssh-key, --ask-pass or --ssh-password-file")
			}
			o.auth.ask = true
			n.SSHAuth = "password"
		}
	}
	if o.lock != "" {
		cfg.Spec.ReleaseLock = o.lock
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("cannot resolve working directory")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot resolve home directory")
	}
	if err := cfg.Normalize(cwd, home); err != nil {
		return nil, err
	}
	// The normalized lock reference is also used by the shared lock reader.
	o.lock = filepath.Clean(cfg.Spec.ReleaseLock)
	return cfg, nil
}

func confirmOperation(env *Env, summary string) bool {
	fmt.Fprintln(env.Stderr, summary)
	input, ok := env.Stdin.(*os.File)
	if !ok || !term.IsTerminal(int(input.Fd())) {
		return false
	}
	fmt.Fprint(env.Stderr, "Type yes to continue: ")
	line, err := bufio.NewReader(input).ReadString('\n')
	return err == nil && strings.TrimSpace(line) == "yes"
}

// runInstallExecution renders the locked chart and applies it through the gated
// platform pipeline. It is only reached once the evidence gate has passed; the
// pipeline enforces the gate again with the caller-supplied evidence.
func runInstallExecution(ctx context.Context, env *Env, store toolkit.RunStore, writer string, epoch int64, cfg *toolkit.DeploymentConfig, lock *toolkit.ReleaseLock, opts installOptions) error {
	bindings, err := os.ReadFile(opts.bindings)
	if err != nil {
		return fmt.Errorf("cannot read bindings: %w", err)
	}
	rendered, err := toolkit.RenderLockedChart(ctx, cfg, lock, opts.chartPackage, bindings, "full")
	if err != nil {
		return fmt.Errorf("cannot render platform release: %w", err)
	}
	workloads, err := parseWorkloads(opts.workloads)
	if err != nil {
		return err
	}
	result, err := toolkit.RunPlatformInstall(ctx, store, writer, epoch, cfg,
		cfg.Spec.Cluster.KubeconfigFile, rendered, workloads, toolkit.KubectlRunner{}, toolkit.CurrentEvidence)
	if err != nil {
		return err
	}
	if !result.Applied || !result.Verified {
		return fmt.Errorf("platform install incomplete")
	}
	if opts.migrationsDir != "" {
		planes, err := buildMigrationPlanes(ctx, cfg, opts.migrationsDir)
		if err != nil {
			return err
		}
		if _, err := toolkit.RunInstallMigrations(ctx, store, writer, epoch, cfg, planes, nil, toolkit.CurrentEvidence); err != nil {
			return err
		}
	}
	return nil
}

// buildMigrationPlanes resolves each configured plane's DSN from its cluster
// Secret. DSNs are used only to apply migrations and are never printed.
func buildMigrationPlanes(ctx context.Context, cfg *toolkit.DeploymentConfig, dir string) ([]toolkit.MigrationPlane, error) {
	runner := toolkit.KubectlRunner{}
	kubeconfig := cfg.Spec.Cluster.KubeconfigFile
	namespace := cfg.Spec.Platform.Namespace
	var planes []toolkit.MigrationPlane
	for _, plane := range cfg.Spec.Platform.Infrastructure.Planes {
		owner, ok := toolkit.MigrationOwnerForPlane(plane.Name)
		if !ok {
			return nil, fmt.Errorf("unknown plane %q", plane.Name)
		}
		secret := plane.PostgreSQLSecretRef
		if idx := strings.LastIndex(secret, "/"); idx >= 0 {
			secret = secret[idx+1:]
		}
		dsn, err := toolkit.ReadSecretKey(ctx, runner, kubeconfig, namespace, secret, "dsn")
		if err != nil {
			return nil, fmt.Errorf("plane %s: %w", plane.Name, err)
		}
		planes = append(planes, toolkit.MigrationPlane{Owner: owner, MigrationsDir: filepath.Join(dir, string(owner)), DSN: dsn})
	}
	return planes, nil
}

func parseWorkloads(values []string) ([]toolkit.WorkloadRef, error) {
	var workloads []toolkit.WorkloadRef
	for _, value := range values {
		kind, name, ok := strings.Cut(value, "/")
		if !ok || kind == "" || name == "" {
			return nil, fmt.Errorf("--workload must be Kind/name")
		}
		workloads = append(workloads, toolkit.WorkloadRef{Kind: kind, Name: name})
	}
	return workloads, nil
}

// runBootstrapExecution is the gated Day-0 path for a blank management node. It
// owns its run directory (the management adapter creates the run), enforces the
// delivery evidence gate first, and performs no platform release install.
func runBootstrapExecution(ctx context.Context, env *Env, cfg *toolkit.DeploymentConfig, lock *toolkit.ReleaseLock, opts installOptions) Result {
	if reasons := toolkit.EvidenceGate(cfg, toolkit.CurrentEvidence); len(reasons) > 0 {
		return fail(ReasonPlatformNotVerified, "bootstrap execution refused: "+strings.Join(reasons, "; "))
	}
	sources := toolkit.BootstrapSources{
		ImageRepository: opts.imageRepository, SandboxImage: opts.sandboxImage,
		CNIImageRepository: opts.cniImageRepository, HelmChartRepository: opts.chartRepository,
	}
	runsDir := opts.runsDir
	if runsDir == "" {
		var err error
		runsDir, err = defaultRunsDir()
		if err != nil {
			return fail(ReasonInvalidConfig, err.Error())
		}
	}
	runID, err := toolkit.NewRunID()
	if err != nil {
		return fail(ReasonOutputFailed, "cannot allocate run identity")
	}
	runDir := filepath.Join(runsDir, runID)
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return fail(ReasonOutputFailed, "cannot create private run directory")
	}
	runner, err := opts.auth.runner(env, cfg)
	if err != nil {
		return fail(ReasonInvalidConfig, err.Error())
	}
	result, err := toolkit.RunManagementBootstrap(ctx, cfg, lock, sources, runID, runDir, runner, toolkit.CurrentEvidence)
	if err != nil {
		return failf(ReasonPlatformNotVerified, "%v", err)
	}
	if result.KubeconfigFile != "" {
		fmt.Fprintf(env.Stdout, "management=%s clusterReady=%v managementKubeconfig=%s\n", result.RunID, result.ClusterReady, result.KubeconfigFile)
	}
	return success("")
}
