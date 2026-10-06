package worker

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/F31/hnb/pkg/kms"
	"github.com/lib/pq"
	"github.com/nats-io/nats.go"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Request is the payload extension-controller publishes on
// hnb.extension.provider.<provider>.<action>.
type Request struct {
	ExtensionID string `json:"extension_id,omitempty"`
	Name        string `json:"name"`
	Version     string `json:"version,omitempty"`
	PrevVersion string `json:"prev_version,omitempty"`
	Provider    string `json:"provider,omitempty"`
	Action      string `json:"action,omitempty"`
	TargetID    string `json:"target_id,omitempty"`
	// Chart optionally overrides the default chart reference (a remote repo
	// name such as "cilium/cilium" or a local path). When empty the built-in
	// catalog mapping is used.
	Chart        string         `json:"chart,omitempty"`
	ChartVersion string         `json:"chartVersion,omitempty"`
	RepoName     string         `json:"repoName,omitempty"`
	RepoURL      string         `json:"repoURL,omitempty"`
	Namespace    string         `json:"namespace,omitempty"`
	Values       map[string]any `json:"values,omitempty"`
	ValuesSchema map[string]any `json:"valuesSchema,omitempty"`
	Preflight    map[string]any `json:"preflight,omitempty"`
}

// Response is the reply extension-controller awaits; status must be
// "succeeded" for the extension to reach a ready/uninstalled state.
type Response struct {
	Status  string       `json:"status"`
	Message string       `json:"message,omitempty"`
	Stage   string       `json:"stage,omitempty"`
	Stages  []StageEvent `json:"stages,omitempty"`
}

type StageEvent struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type Worker struct {
	db            *sql.DB
	cipher        kms.Decrypter
	nc            *nats.Conn
	helmPath      string
	kubeconfigDir string
	dryRun        bool
}

var helmNamePattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

type targetCredential struct {
	tenantID          string
	targetType        string
	status            string
	lifecycleState    string
	connectivityState string
	scope             string
	name              string
	capability        targetCapability
}

type targetCapability struct {
	kubeVersion string
	cpuCores    int
	memoryMB    int64
	storageGB   int64
	cniPlugins  []string
	csiDrivers  []string
	gpuCount    int
	features    map[string]bool
}

// New builds a worker. cipher may be nil when no master key is configured, in
// which case installs fail closed with a clear error.
func New(db *sql.DB, cipher kms.Decrypter, nc *nats.Conn, helmPath, kubeconfigDir string, dryRun bool) *Worker {
	return &Worker{
		db:            db,
		cipher:        cipher,
		nc:            nc,
		helmPath:      helmPath,
		kubeconfigDir: kubeconfigDir,
		dryRun:        dryRun,
	}
}

// Start subscribes to the extension provider lifecycle subjects.
func (w *Worker) Start(ctx context.Context) error {
	if w.cipher == nil {
		log.Printf("[plugin-worker] WARNING: no HNB_MASTER_KEY configured; plugin installs will fail closed")
	}
	if w.dryRun {
		log.Printf("[plugin-worker] dry-run mode enabled; provider requests will be acknowledged without helm changes")
	} else if _, err := exec.LookPath(w.helmPath); err != nil {
		return fmt.Errorf("helm binary %q is not available: %w", w.helmPath, err)
	}
	subs := map[string]func(*nats.Msg){
		"hnb.extension.provider.*.install":   w.handle,
		"hnb.extension.provider.*.upgrade":   w.handle,
		"hnb.extension.provider.*.uninstall": w.handle,
		"hnb.extension.provider.*.health":    w.handle,
	}
	for subject := range subs {
		s := subject
		if _, err := w.nc.Subscribe(s, func(msg *nats.Msg) {
			w.handle(msg)
		}); err != nil {
			return fmt.Errorf("subscribe %s: %w", s, err)
		}
		log.Printf("[plugin-worker] subscribed to %s", s)
	}
	<-ctx.Done()
	return nil
}

func (w *Worker) handle(msg *nats.Msg) {
	var req Request
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		w.respond(msg, Response{Status: "failed", Message: fmt.Sprintf("invalid request: %v", err)})
		return
	}
	log.Printf("[plugin-worker] %s/%s provider=%s target=%s version=%s", req.Action, req.Name, req.Provider, req.TargetID, req.Version)

	var resp Response
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	switch req.Action {
	case "install", "upgrade":
		resp = w.Install(ctx, &req)
	case "uninstall":
		resp = w.Uninstall(ctx, &req)
	case "health":
		resp = w.Health(ctx, &req)
	default:
		resp = Response{Status: "failed", Message: fmt.Sprintf("unsupported action %q", req.Action)}
	}
	if resp.Status != "succeeded" {
		log.Printf("[plugin-worker] %s/%s failed: %s", req.Action, req.Name, resp.Message)
	}
	w.respond(msg, resp)
}

// kubeconfigFor resolves the target cluster's kubeconfig via its credential_ref
// (or the tenant's most recent kubeconfig secret) and returns a file path
// containing the plaintext kubeconfig.
func (w *Worker) kubeconfigFor(req *Request) (string, error) {
	if w.cipher == nil {
		return "", fmt.Errorf("master key not configured")
	}
	var encrypted, candidate string
	target, err := w.targetCredentialFor(req.TargetID)
	if err != nil {
		return "", err
	}
	if err := validateTargetForHelm(target); err != nil {
		return "", err
	}
	if err := validateTargetPreflight(target.capability, req.Preflight); err != nil {
		return "", err
	}
	if target.scope != "" && target.name != "" {
		err = w.db.QueryRow(`
			SELECT sr.encrypted_value FROM secret_references sr
			JOIN kms_providers kp ON kp.id = sr.kms_provider_id AND kp.is_active
			WHERE sr.tenant_id = $1 AND sr.scope = $2 AND sr.name = $3
			  AND sr.purpose = 'kubeconfig' AND sr.is_active
			ORDER BY sr.created_at DESC LIMIT 1`, target.tenantID, target.scope, target.name).Scan(&encrypted)
		if err == nil {
			return w.writeKubeconfig(req.TargetID, encrypted)
		}
		log.Printf("[plugin-worker] credential_ref secret missing (%s/%s): %v", target.scope, target.name, err)
	}
	err = w.db.QueryRow(`
		SELECT sr.encrypted_value FROM secret_references sr
		JOIN kms_providers kp ON kp.id = sr.kms_provider_id AND kp.is_active
		WHERE sr.tenant_id = $1 AND sr.purpose = 'kubeconfig'
		  AND sr.is_active
		ORDER BY sr.created_at DESC LIMIT 1`, target.tenantID).Scan(&candidate)
	if err != nil {
		return "", fmt.Errorf("no kubeconfig secret for tenant %s: %w", target.tenantID, err)
	}
	return w.writeKubeconfig(req.TargetID, candidate)
}

func (w *Worker) targetCredentialFor(targetID string) (targetCredential, error) {
	if w.db == nil {
		return targetCredential{}, fmt.Errorf("database not configured")
	}
	var target targetCredential
	// Exact credential_ref preferred (same semantics as platform-api
	// KubeConfigEncryptedForRef); legacy targets fall back to the tenant's
	// most recent kubeconfig secret.
	err := w.db.QueryRow(`
		SELECT rt.tenant_id,
		       COALESCE(rt.target_type, ''),
		       COALESCE(rt.status, ''),
		       COALESCE(rt.lifecycle_state, ''),
		       COALESCE(rt.connectivity_state, ''),
		       COALESCE(rt.credential_ref->>'scope','') ,
		       COALESCE(rt.credential_ref->>'name','')
		FROM runtime_targets rt WHERE rt.id = $1::uuid`, targetID).Scan(&target.tenantID, &target.targetType, &target.status, &target.lifecycleState, &target.connectivityState, &target.scope, &target.name)
	if err != nil {
		return targetCredential{}, fmt.Errorf("load target: %w", err)
	}
	if err := w.loadLatestCapability(&target, targetID); err != nil {
		return targetCredential{}, err
	}
	return target, nil
}

func (w *Worker) loadLatestCapability(target *targetCredential, targetID string) error {
	var cniPlugins, csiDrivers pq.StringArray
	var featuresRaw []byte
	err := w.db.QueryRow(`
		SELECT COALESCE(kube_version, ''), COALESCE(cpu_cores, 0), COALESCE(memory_mb, 0), COALESCE(storage_gb, 0),
		       COALESCE(cni_plugins, '{}'), COALESCE(csi_drivers, '{}'), COALESCE(gpu_count, 0), COALESCE(features, '{}')
		FROM capability_snapshots
		WHERE target_id = $1::uuid
		ORDER BY observed_at DESC, created_at DESC LIMIT 1`, targetID).
		Scan(&target.capability.kubeVersion, &target.capability.cpuCores, &target.capability.memoryMB, &target.capability.storageGB, &cniPlugins, &csiDrivers, &target.capability.gpuCount, &featuresRaw)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load target capability: %w", err)
	}
	target.capability.cniPlugins = []string(cniPlugins)
	target.capability.csiDrivers = []string(csiDrivers)
	if len(featuresRaw) > 0 {
		if err := json.Unmarshal(featuresRaw, &target.capability.features); err != nil {
			return fmt.Errorf("decode target features: %w", err)
		}
	}
	return nil
}

func validateTargetPreflight(capability targetCapability, preflight map[string]any) error {
	if len(preflight) == 0 {
		return nil
	}
	if minVersion, _ := preflight["kubernetesMinVersion"].(string); minVersion != "" {
		if capability.kubeVersion == "" {
			return fmt.Errorf("preflight: target Kubernetes version is unknown")
		}
		if compareKubeVersion(capability.kubeVersion, minVersion) < 0 {
			return fmt.Errorf("preflight: Kubernetes version %s is below required %s", capability.kubeVersion, minVersion)
		}
	}
	if maxVersion, _ := preflight["kubernetesMaxVersion"].(string); maxVersion != "" {
		if capability.kubeVersion == "" {
			return fmt.Errorf("preflight: target Kubernetes version is unknown")
		}
		if compareKubeVersion(capability.kubeVersion, maxVersion) > 0 {
			return fmt.Errorf("preflight: Kubernetes version %s is above supported %s", capability.kubeVersion, maxVersion)
		}
	}
	if required, _ := preflight["requiresGPU"].(bool); required && capability.gpuCount <= 0 {
		return fmt.Errorf("preflight: GPU is required")
	}
	if minCPU, ok := intConstraint(preflight["minCPUCores"]); ok && capability.cpuCores < minCPU {
		return fmt.Errorf("preflight: CPU cores %d below required %d", capability.cpuCores, minCPU)
	}
	if minMemory, ok := int64Constraint(preflight["minMemoryMB"]); ok && capability.memoryMB < minMemory {
		return fmt.Errorf("preflight: memory %dMB below required %dMB", capability.memoryMB, minMemory)
	}
	if minStorage, ok := int64Constraint(preflight["minStorageGB"]); ok && capability.storageGB < minStorage {
		return fmt.Errorf("preflight: storage %dGB below required %dGB", capability.storageGB, minStorage)
	}
	for _, plugin := range stringList(preflight["requiredCNIPlugins"]) {
		if !containsString(capability.cniPlugins, plugin) {
			return fmt.Errorf("preflight: CNI plugin %q is required", plugin)
		}
	}
	for _, driver := range stringList(preflight["requiredCSIDrivers"]) {
		if !containsString(capability.csiDrivers, driver) {
			return fmt.Errorf("preflight: CSI driver %q is required", driver)
		}
	}
	for _, feature := range stringList(preflight["requiredFeatures"]) {
		if !capability.features[feature] {
			return fmt.Errorf("preflight: feature %q is required", feature)
		}
	}
	return nil
}

func helmServerDryRunEnabled(preflight map[string]any) bool {
	enabled, _ := preflight["helmServerDryRun"].(bool)
	return enabled
}

func validateTargetForHelm(target targetCredential) error {
	if target.targetType != "kubernetes" {
		return fmt.Errorf("target type %q does not support helm plugins", target.targetType)
	}
	switch strings.ToLower(target.status) {
	case "offline", "decommissioned":
		return fmt.Errorf("target status %q does not support helm plugins", target.status)
	}
	if strings.EqualFold(target.lifecycleState, "TERMINATED") {
		return fmt.Errorf("target lifecycle_state %q does not support helm plugins", target.lifecycleState)
	}
	if strings.EqualFold(target.connectivityState, "DISCONNECTED") {
		return fmt.Errorf("target connectivity_state %q does not support helm plugins", target.connectivityState)
	}
	return nil
}

func (w *Worker) writeKubeconfig(targetID, sealed string) (string, error) {
	decrypted, err := w.cipher.Decrypt(sealed)
	if err != nil {
		return "", fmt.Errorf("decrypt kubeconfig: %w", err)
	}
	if err := os.MkdirAll(w.kubeconfigDir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(w.kubeconfigDir, "kubeconfig-"+targetID+".yaml")
	if err := os.WriteFile(path, decrypted, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func (w *Worker) Install(ctx context.Context, req *Request) Response {
	if w.dryRun {
		return stageSuccess([]StageEvent{{Name: "dry-run", Status: "succeeded", Message: "install acknowledged"}})
	}
	stages := make([]StageEvent, 0, 7)
	if err := validateHelmRequest(req, true); err != nil {
		return stageFailure(stages, "validate-request", err.Error())
	}
	stages = append(stages, StageEvent{Name: "validate-request", Status: "succeeded"})
	if err := validateValuesSchema(req.Values, req.ValuesSchema); err != nil {
		return stageFailure(stages, "validate-values", err.Error())
	}
	stages = append(stages, StageEvent{Name: "validate-values", Status: "succeeded"})
	kubeconfig, err := w.kubeconfigFor(req)
	if err != nil {
		return stageFailure(stages, "resolve-kubeconfig", err.Error())
	}
	stages = append(stages, StageEvent{Name: "resolve-kubeconfig", Status: "succeeded"})
	if err := w.ensureRepo(ctx, kubeconfig, req); err != nil {
		return stageFailure(stages, "helm-repo", err.Error())
	}
	if req.RepoURL != "" {
		stages = append(stages, StageEvent{Name: "helm-repo", Status: "succeeded"})
	}
	args, cleanup, err := w.installArgs(req)
	if err != nil {
		return stageFailure(stages, "render-values", err.Error())
	}
	stages = append(stages, StageEvent{Name: "render-values", Status: "succeeded"})
	if cleanup != nil {
		defer cleanup()
	}
	if helmServerDryRunEnabled(req.Preflight) {
		out, err := helmRun(ctx, w.helmPath, kubeconfig, serverDryRunArgs(args)...)
		if err != nil {
			return stageFailure(stages, "helm-server-dry-run", fmt.Sprintf("%v (%s)", err, truncate(out)))
		}
		stages = append(stages, StageEvent{Name: "helm-server-dry-run", Status: "succeeded"})
	}
	out, err := helmRun(ctx, w.helmPath, kubeconfig, args...)
	if err != nil {
		return stageFailure(stages, "helm-install", fmt.Sprintf("%v (%s)", err, truncate(out)))
	}
	stages = append(stages, StageEvent{Name: "helm-install", Status: "succeeded"})
	return stageSuccess(stages)
}

func (w *Worker) Uninstall(ctx context.Context, req *Request) Response {
	if w.dryRun {
		return stageSuccess([]StageEvent{{Name: "dry-run", Status: "succeeded", Message: "uninstall acknowledged"}})
	}
	stages := make([]StageEvent, 0, 3)
	if err := validateHelmRequest(req, false); err != nil {
		return stageFailure(stages, "validate-request", err.Error())
	}
	stages = append(stages, StageEvent{Name: "validate-request", Status: "succeeded"})
	kubeconfig, err := w.kubeconfigFor(req)
	if err != nil {
		return stageFailure(stages, "resolve-kubeconfig", err.Error())
	}
	stages = append(stages, StageEvent{Name: "resolve-kubeconfig", Status: "succeeded"})
	out, err := helmRun(ctx, w.helmPath, kubeconfig, "uninstall", req.Name,
		"--namespace", namespaceFor(req))
	if err != nil {
		// A release that is absent is already uninstalled: treat as success.
		if strings.Contains(err.Error(), "not found") || strings.Contains(out, "not found") {
			stages = append(stages, StageEvent{Name: "helm-uninstall", Status: "succeeded", Message: "release not found (already uninstalled)"})
			return stageSuccess(stages)
		}
		return stageFailure(stages, "helm-uninstall", fmt.Sprintf("%v (%s)", err, truncate(out)))
	}
	stages = append(stages, StageEvent{Name: "helm-uninstall", Status: "succeeded"})
	return stageSuccess(stages)
}

func (w *Worker) Health(ctx context.Context, req *Request) Response {
	if w.dryRun {
		return stageSuccess([]StageEvent{{Name: "dry-run", Status: "succeeded", Message: "health acknowledged"}})
	}
	stages := make([]StageEvent, 0, 3)
	if err := validateHelmRequest(req, false); err != nil {
		return stageFailure(stages, "validate-request", err.Error())
	}
	stages = append(stages, StageEvent{Name: "validate-request", Status: "succeeded"})
	kubeconfig, err := w.kubeconfigFor(req)
	if err != nil {
		return stageFailure(stages, "resolve-kubeconfig", err.Error())
	}
	stages = append(stages, StageEvent{Name: "resolve-kubeconfig", Status: "succeeded"})
	out, err := helmRun(ctx, w.helmPath, kubeconfig, "status", req.Name,
		"--namespace", namespaceFor(req))
	if err != nil {
		return stageFailure(stages, "helm-status", fmt.Sprintf("%v (%s)", err, truncate(out)))
	}
	if !strings.Contains(out, "deployed") {
		return stageFailure(stages, "helm-status", "release not deployed")
	}
	stages = append(stages, StageEvent{Name: "helm-status", Status: "succeeded"})
	message, err := workloadReadiness(ctx, kubeconfig, namespaceFor(req), req.Name)
	if err != nil {
		return stageFailure(stages, "workload-readiness", err.Error())
	}
	stages = append(stages, StageEvent{Name: "workload-readiness", Status: "succeeded", Message: message})
	return stageSuccess(stages)
}

func workloadReadiness(ctx context.Context, kubeconfig, namespace, releaseName string) (string, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return "", fmt.Errorf("build Kubernetes client: %w", err)
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return "", fmt.Errorf("create Kubernetes client: %w", err)
	}
	selector := "app.kubernetes.io/instance=" + releaseName
	deployments, err := client.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return "", fmt.Errorf("list deployments: %w", err)
	}
	for _, deploy := range deployments.Items {
		if err := deploymentReady(deploy); err != nil {
			return "", err
		}
	}
	daemonSets, err := client.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return "", fmt.Errorf("list daemonsets: %w", err)
	}
	for _, ds := range daemonSets.Items {
		if err := daemonSetReady(ds); err != nil {
			return "", err
		}
	}
	statefulSets, err := client.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return "", fmt.Errorf("list statefulsets: %w", err)
	}
	for _, sts := range statefulSets.Items {
		if err := statefulSetReady(sts); err != nil {
			return "", err
		}
	}
	pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return "", fmt.Errorf("list pods: %w", err)
	}
	for _, pod := range pods.Items {
		if err := podReady(pod); err != nil {
			return "", err
		}
	}
	total := len(deployments.Items) + len(daemonSets.Items) + len(statefulSets.Items) + len(pods.Items)
	if total == 0 {
		return "no labelled workloads found", nil
	}
	return fmt.Sprintf("%d labelled workloads ready", total), nil
}

func deploymentReady(deploy appsv1.Deployment) error {
	desired := int32(1)
	if deploy.Spec.Replicas != nil {
		desired = *deploy.Spec.Replicas
	}
	if deploy.Status.ObservedGeneration < deploy.Generation {
		return fmt.Errorf("deployment %s generation not observed", deploy.Name)
	}
	if deploy.Status.UpdatedReplicas < desired || deploy.Status.AvailableReplicas < desired {
		return fmt.Errorf("deployment %s not ready: updated=%d available=%d desired=%d", deploy.Name, deploy.Status.UpdatedReplicas, deploy.Status.AvailableReplicas, desired)
	}
	return nil
}

func daemonSetReady(ds appsv1.DaemonSet) error {
	desired := ds.Status.DesiredNumberScheduled
	if ds.Status.ObservedGeneration < ds.Generation {
		return fmt.Errorf("daemonset %s generation not observed", ds.Name)
	}
	if ds.Status.UpdatedNumberScheduled < desired || ds.Status.NumberAvailable < desired {
		return fmt.Errorf("daemonset %s not ready: updated=%d available=%d desired=%d", ds.Name, ds.Status.UpdatedNumberScheduled, ds.Status.NumberAvailable, desired)
	}
	return nil
}

func statefulSetReady(sts appsv1.StatefulSet) error {
	desired := int32(1)
	if sts.Spec.Replicas != nil {
		desired = *sts.Spec.Replicas
	}
	if sts.Status.ObservedGeneration < sts.Generation {
		return fmt.Errorf("statefulset %s generation not observed", sts.Name)
	}
	if sts.Status.UpdatedReplicas < desired || sts.Status.ReadyReplicas < desired {
		return fmt.Errorf("statefulset %s not ready: updated=%d ready=%d desired=%d", sts.Name, sts.Status.UpdatedReplicas, sts.Status.ReadyReplicas, desired)
	}
	return nil
}

func podReady(pod corev1.Pod) error {
	if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodSucceeded {
		return fmt.Errorf("pod %s phase is %s", pod.Name, pod.Status.Phase)
	}
	if pod.Status.Phase == corev1.PodSucceeded {
		return nil
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return nil
		}
	}
	return fmt.Errorf("pod %s is not ready", pod.Name)
}

func stageSuccess(stages []StageEvent) Response {
	stage := lastStageName(stages)
	message := "succeeded"
	if stage != "" {
		message = stage + " succeeded"
	}
	return Response{Status: "succeeded", Stage: stage, Message: message, Stages: stages}
}

func stageFailure(stages []StageEvent, stage, message string) Response {
	stages = append(stages, StageEvent{Name: stage, Status: "failed", Message: message})
	return Response{Status: "failed", Stage: stage, Message: stage + ": " + message, Stages: stages}
}

func lastStageName(stages []StageEvent) string {
	if len(stages) == 0 {
		return ""
	}
	return stages[len(stages)-1].Name
}

func helmRun(ctx context.Context, helmPath, kubeconfig string, args ...string) (string, error) {
	full := []string{"--kubeconfig", kubeconfig}
	full = append(full, args...)
	cmd := exec.CommandContext(ctx, helmPath, full...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String() + stderr.String(), err
	}
	return stdout.String() + stderr.String(), nil
}

func (w *Worker) respond(msg *nats.Msg, resp Response) {
	data, _ := json.Marshal(resp)
	if err := msg.Respond(data); err != nil {
		log.Printf("[plugin-worker] respond error: %v", err)
	}
}

func (w *Worker) ensureRepo(ctx context.Context, kubeconfig string, req *Request) error {
	if req.RepoURL == "" {
		return nil
	}
	repoName := req.RepoName
	if repoName == "" {
		repoName = req.Name
	}
	if out, err := helmRun(ctx, w.helmPath, kubeconfig, "repo", "add", repoName, req.RepoURL, "--force-update"); err != nil {
		return fmt.Errorf("helm repo add: %v (%s)", err, truncate(out))
	}
	if out, err := helmRun(ctx, w.helmPath, kubeconfig, "repo", "update", repoName); err != nil {
		return fmt.Errorf("helm repo update: %v (%s)", err, truncate(out))
	}
	return nil
}

func validateHelmRequest(req *Request, requireChart bool) error {
	if req == nil {
		return fmt.Errorf("request is required")
	}
	if !validHelmName(req.Name, 53) {
		return fmt.Errorf("invalid release name %q", req.Name)
	}
	if req.TargetID == "" {
		return fmt.Errorf("target_id is required")
	}
	if req.Namespace != "" && !validHelmName(req.Namespace, 63) {
		return fmt.Errorf("invalid namespace %q", req.Namespace)
	}
	if req.RepoName != "" && !validHelmName(req.RepoName, 63) {
		return fmt.Errorf("invalid repoName %q", req.RepoName)
	}
	if req.RepoURL != "" {
		repoURL, err := url.Parse(req.RepoURL)
		if err != nil || repoURL.Host == "" || (repoURL.Scheme != "https" && repoURL.Scheme != "http") || repoURL.User != nil {
			return fmt.Errorf("invalid repoURL %q", req.RepoURL)
		}
	}
	if req.Chart != "" && !validChartRef(req.Chart) {
		return fmt.Errorf("invalid chart %q", req.Chart)
	}
	if req.ChartVersion != "" && strings.ContainsAny(req.ChartVersion, " \t\n\r") {
		return fmt.Errorf("invalid chartVersion %q", req.ChartVersion)
	}
	if requireChart && !hasInstallableChart(req) {
		return fmt.Errorf("chart is required")
	}
	return nil
}

func validHelmName(name string, maxLen int) bool {
	return len(name) > 0 && len(name) <= maxLen && helmNamePattern.MatchString(name)
}

func validChartRef(chart string) bool {
	if strings.ContainsAny(chart, " \t\n\r") {
		return false
	}
	if isLocalChart(chart) {
		return !strings.Contains(filepath.Clean(chart), "..")
	}
	parts := strings.Split(chart, "/")
	if len(parts) > 2 {
		return false
	}
	for _, part := range parts {
		if !validHelmName(part, 128) {
			return false
		}
	}
	return true
}

func validateValuesSchema(values map[string]any, schema map[string]any) error {
	if len(schema) == 0 {
		return nil
	}
	return validateValueAgainstSchema(values, schema, "values")
}

func validateValueAgainstSchema(value any, schema map[string]any, path string) error {
	if expectedType, _ := schema["type"].(string); expectedType != "" && !valueMatchesType(value, expectedType) {
		return fmt.Errorf("helm values schema: %s must be %s", path, expectedType)
	}
	if enum, ok := schema["enum"]; ok && !enumContains(enum, value) {
		return fmt.Errorf("helm values schema: %s must be one of %v", path, enum)
	}
	if err := validateScalarConstraints(value, schema, path); err != nil {
		return err
	}
	objectValue, isObject := value.(map[string]any)
	if !isObject {
		return nil
	}
	if err := validateRequiredValues(objectValue, schema, path); err != nil {
		return err
	}
	return validateValueProperties(objectValue, schema, path)
}

func validateRequiredValues(values map[string]any, schema map[string]any, path string) error {
	if required, ok := schema["required"]; ok {
		for _, key := range stringList(required) {
			if _, exists := values[key]; !exists {
				return fmt.Errorf("helm values schema: %s.%s is required", path, key)
			}
		}
	}
	return nil
}

func validateValueProperties(values map[string]any, schema map[string]any, path string) error {
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		properties = map[string]any{}
	}
	if additional, ok := schema["additionalProperties"].(bool); ok && !additional {
		for key := range values {
			if _, allowed := properties[key]; !allowed {
				return fmt.Errorf("helm values schema: %s.%s is not allowed", path, key)
			}
		}
	}
	for key, rawProperty := range properties {
		property, ok := rawProperty.(map[string]any)
		if !ok {
			continue
		}
		value, exists := values[key]
		if !exists {
			continue
		}
		currentPath := path + "." + key
		if err := validateValueAgainstSchema(value, property, currentPath); err != nil {
			return err
		}
	}
	return nil
}

func validateScalarConstraints(value any, schema map[string]any, path string) error {
	if text, ok := value.(string); ok {
		if min, ok := numericConstraint(schema["minLength"]); ok && float64(len(text)) < min {
			return fmt.Errorf("helm values schema: %s length must be >= %v", path, trimFloat(min))
		}
		if max, ok := numericConstraint(schema["maxLength"]); ok && float64(len(text)) > max {
			return fmt.Errorf("helm values schema: %s length must be <= %v", path, trimFloat(max))
		}
		if pattern, _ := schema["pattern"].(string); pattern != "" {
			re, err := regexp.Compile(pattern)
			if err != nil {
				return fmt.Errorf("helm values schema: %s has invalid pattern %q", path, pattern)
			}
			if !re.MatchString(text) {
				return fmt.Errorf("helm values schema: %s must match %q", path, pattern)
			}
		}
	}
	if number, ok := numericValue(value); ok {
		if min, ok := numericConstraint(schema["minimum"]); ok && number < min {
			return fmt.Errorf("helm values schema: %s must be >= %v", path, trimFloat(min))
		}
		if max, ok := numericConstraint(schema["maximum"]); ok && number > max {
			return fmt.Errorf("helm values schema: %s must be <= %v", path, trimFloat(max))
		}
	}
	return nil
}

func enumContains(enum any, value any) bool {
	items, ok := enum.([]any)
	if !ok {
		return true
	}
	for _, item := range items {
		if fmt.Sprint(item) == fmt.Sprint(value) {
			return true
		}
	}
	return false
}

func numericConstraint(value any) (float64, bool) {
	return numericValue(value)
}

func numericValue(value any) (float64, bool) {
	switch n := value.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint64:
		return float64(n), true
	case uint32:
		return float64(n), true
	default:
		return 0, false
	}
}

func trimFloat(value float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", value), "0"), ".")
}

func intConstraint(value any) (int, bool) {
	n, ok := numericValue(value)
	if !ok {
		return 0, false
	}
	return int(n), true
}

func int64Constraint(value any) (int64, bool) {
	n, ok := numericValue(value)
	if !ok {
		return 0, false
	}
	return int64(n), true
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func compareKubeVersion(left, right string) int {
	leftParts := parseKubeVersion(left)
	rightParts := parseKubeVersion(right)
	for i := 0; i < len(leftParts); i++ {
		if leftParts[i] < rightParts[i] {
			return -1
		}
		if leftParts[i] > rightParts[i] {
			return 1
		}
	}
	return 0
}

func parseKubeVersion(version string) [3]int {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	parts := strings.Split(version, ".")
	var out [3]int
	for i := 0; i < len(out) && i < len(parts); i++ {
		part := parts[i]
		for j, r := range part {
			if r < '0' || r > '9' {
				part = part[:j]
				break
			}
		}
		if part == "" {
			continue
		}
		_, _ = fmt.Sscanf(part, "%d", &out[i])
	}
	return out
}

func stringList(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func valueMatchesType(value any, expected string) bool {
	switch expected {
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "number":
		switch value.(type) {
		case float64, float32, int, int64, int32, uint, uint64, uint32:
			return true
		default:
			return false
		}
	case "integer":
		switch n := value.(type) {
		case int, int64, int32, uint, uint64, uint32:
			return true
		case float64:
			return n == float64(int64(n))
		default:
			return false
		}
	default:
		return true
	}
}

func (w *Worker) installArgs(req *Request) ([]string, func(), error) {
	args := []string{"upgrade", "--install", req.Name, chartArg(req),
		"--namespace", namespaceFor(req),
		"--create-namespace",
		"--wait", "--timeout", "15m"}
	// --version only applies to remote chart references; local chart paths
	// carry their own version in Chart.yaml.
	chartVersion := req.ChartVersion
	if chartVersion == "" && req.Chart == "" {
		chartVersion = req.Version
	}
	if chartVersion != "" && !isLocalChart(req.Chart) {
		args = append(args, "--version", chartVersion)
	}
	if len(req.Values) == 0 {
		return args, nil, nil
	}
	path, err := w.writeValues(req)
	if err != nil {
		return nil, nil, err
	}
	args = append(args, "--values", path)
	return args, func() { _ = os.Remove(path) }, nil
}

func serverDryRunArgs(args []string) []string {
	out := append([]string{}, args...)
	out = append(out, "--dry-run=server", "--debug")
	return out
}

func (w *Worker) writeValues(req *Request) (string, error) {
	if err := os.MkdirAll(w.kubeconfigDir, 0o700); err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(req.Values, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal helm values: %w", err)
	}
	path := filepath.Join(w.kubeconfigDir, "values-"+req.Name+"-"+req.TargetID+".json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// chartArg resolves the chart reference to install: an explicit request
// override (local path or remote repo name) wins, otherwise the built-in
// catalog mapping for the plugin name is used.
func chartArg(req *Request) string {
	if req.Chart != "" {
		if req.RepoName != "" && !strings.Contains(req.Chart, "/") && !strings.HasPrefix(req.Chart, ".") && !filepath.IsAbs(req.Chart) {
			return req.RepoName + "/" + req.Chart
		}
		return req.Chart
	}
	return chartName(req.Name)
}

func hasInstallableChart(req *Request) bool {
	if req.Chart != "" {
		return true
	}
	return knownChartName(req.Name) != ""
}

func isLocalChart(chart string) bool {
	return chart != "" && (strings.HasPrefix(chart, ".") || strings.HasPrefix(chart, "/"))
}

func chartName(name string) string {
	if chart := knownChartName(name); chart != "" {
		return chart
	}
	return name + "/" + name
}

func knownChartName(name string) string {
	// The platform catalog uses product keys that map to upstream helm charts.
	switch name {
	case "cilium":
		return "cilium/cilium"
	case "calico":
		return "projectcalico/tigera-operator"
	case "kube-ovn":
		return "kubeovn/kube-ovn"
	case "prometheus-operator":
		return "prometheus-community/kube-prometheus-stack"
	case "rook-ceph":
		return "rook-release/rook-ceph"
	case "longhorn":
		return "longhorn/longhorn"
	case "falco":
		return "falcosecurity/falco"
	case "keda":
		return "kedacore/keda"
	case "karmada":
		return "karmada/karmada-operator"
	case "hami":
		return "projecthami/hami"
	case "gpu-operator":
		return "nvidia/gpu-operator"
	default:
		return ""
	}
}

func namespaceFor(req *Request) string {
	if req.Namespace != "" {
		return req.Namespace
	}
	switch req.Name {
	case "cilium", "calico", "kube-ovn":
		return "kube-system"
	case "longhorn":
		return "longhorn-system"
	case "falco":
		return "falco"
	case "keda":
		return "keda"
	case "rook-ceph":
		return "rook-ceph"
	default:
		return "kp-default"
	}
}

func truncate(s string) string {
	if len(s) > 400 {
		return s[:400] + "..."
	}
	return s
}
