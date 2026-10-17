package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/F31/hnb/cmd/runtime-target-lifecycle-provider/internal/provision"
)

const (
	kubernetesManagedByAnnotation = "hnb.io/managed-by"
	kubernetesProviderValue       = "hnb-kubernetes-lifecycle-provider"
	kubernetesTenantAnnotation    = "hnb.io/tenant-id"
	kubernetesTargetAnnotation    = "hnb.io/target-id"
	kubernetesOperationAnnotation = "hnb.io/operation-id"
	kubernetesFencingAnnotation   = "hnb.io/fencing-generation"
	kubernetesVersionAnnotation   = "hnb.io/desired-version"
	kubernetesStepAnnotation      = "hnb.io/step-id"
	kubernetesIdemAnnotation      = "hnb.io/idempotency-key"
	kubernetesAttemptAnnotation   = "hnb.io/execution-attempt-id"
	kubernetesActionAnnotation    = "hnb.io/action"
	// kubernetesEndpointAnnotation records the provisioned control-plane
	// endpoint so later actions (node add, upgrade) can recover it after a
	// provider restart without re-bootstrapping anything.
	kubernetesEndpointAnnotation = "hnb.io/control-plane-endpoint"
	// kubernetesKubeconfigRefAnnotation records where the admin kubeconfig was
	// stored so the provider can reopen the target record after a restart.
	kubernetesKubeconfigRefAnnotation = "hnb.io/kubeconfig-ref"
)

// KubernetesManager implements LifecycleManager for KubernetesRuntimeTarget
// lifecycle actions (5.5/5.6). It manages a namespace per target to track
// lifecycle state, using k8s annotations for fencing/fencing and idempotency.
type KubernetesManager struct {
	profile    Profile
	mu         sync.Mutex
	targets    map[string]targetRecord
	clientFunc func(kubeconfig []byte) (kubernetes.Interface, error)
	client     kubernetes.Interface
	cachedKube string
	now        func() time.Time
	// provisioner is used when create carries user-supplied SSH nodes; it
	// bootstraps a real control-plane/workers (CAPI-aligned), stores the
	// resulting kubeconfig, and registers the target. nil disables provisioning.
	provisioner interface {
		Provision(ctx context.Context, spec provision.ProvisionSpec) (provision.Result, error)
		AddNodes(ctx context.Context, spec provision.AddNodesSpec) (provision.AddNodesResult, error)
	}
	// kubeconfigLoader resolves a stored kubeconfig SecretReference back into
	// plaintext so the provider can persist/reopen the target record inside
	// the provisioned cluster. nil disables record persistence (tests).
	kubeconfigLoader func(ctx context.Context, tenantID string, ref provision.SecretReference) ([]byte, error)
}

// NewKubernetesManager creates a manager that uses the resolved kubeconfig
// from SecretResolver to connect to the target Kubernetes cluster.
func NewKubernetesManager(profile Profile) *KubernetesManager {
	return &KubernetesManager{
		profile:    profile,
		targets:    make(map[string]targetRecord),
		clientFunc: realKubernetesClient,
		now:        time.Now,
	}
}

// NewKubernetesManagerWithProvisioner builds a manager that can self-provision
// clusters over user-supplied SSH nodes (create with sshSecretRef + nodes).
func NewKubernetesManagerWithProvisioner(profile Profile, provisioner interface {
	Provision(ctx context.Context, spec provision.ProvisionSpec) (provision.Result, error)
	AddNodes(ctx context.Context, spec provision.AddNodesSpec) (provision.AddNodesResult, error)
}) *KubernetesManager {
	m := NewKubernetesManager(profile)
	m.provisioner = provisioner
	return m
}

// NewKubernetesManagerWithProvisionerAndLoader additionally enables persisting
// the target record into the provisioned cluster (managed namespace), so a
// provider restart can recover the control-plane endpoint and kubeconfig
// reference for follow-up actions such as node add.
func NewKubernetesManagerWithProvisionerAndLoader(profile Profile, provisioner interface {
	Provision(ctx context.Context, spec provision.ProvisionSpec) (provision.Result, error)
	AddNodes(ctx context.Context, spec provision.AddNodesSpec) (provision.AddNodesResult, error)
}, loader func(ctx context.Context, tenantID string, ref provision.SecretReference) ([]byte, error)) *KubernetesManager {
	m := NewKubernetesManagerWithProvisioner(profile, provisioner)
	m.kubeconfigLoader = loader
	return m
}

// NewKubernetesManagerWithClient allows injecting a fake client for testing.
func NewKubernetesManagerWithClient(profile Profile, clientFunc func(kubeconfig []byte) (kubernetes.Interface, error)) *KubernetesManager {
	return &KubernetesManager{
		profile:    profile,
		targets:    make(map[string]targetRecord),
		clientFunc: clientFunc,
		now:        time.Now,
	}
}

// Apply implements LifecycleManager with real Kubernetes operations.
// For "create" (provision-and-register): creates a managed namespace.
// For "import" (register): validates connectivity without modifying resources.
// For "upgrade": validates the target and updates the managed version.
// For "unmanage": cleans up the managed namespace (not non-Operation-owned resources).
func (m *KubernetesManager) Apply(ctx context.Context, execution ExecutionContext, input LifecycleInput) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, targetError("apply lifecycle action", err)
	}

	if err := m.validateFencing(execution, input); err != nil {
		return Result{}, err
	}

	m.mu.Lock()
	record, ok := m.targets[input.TargetID]
	m.mu.Unlock()
	// Self-hosted provisioning has no kubeconfig before it boots the cluster, so
	// skip namespace-record recovery when bootstrapping over SSH.
	provisioningCreate := input.Action == "create" && input.SSHSecretRef != nil && m.provisioner != nil
	if !ok && input.Action != "import" && !provisioningCreate {
		loaded, found, err := m.loadRecordFromNamespace(ctx, execution, input)
		if err != nil {
			return Result{}, err
		}
		if found {
			record, ok = loaded, true
		}
	}

	if ok {
		switch {
		case record.TenantID != execution.TenantID || record.TargetKind != input.TargetKind:
			return Result{}, fail(409, ErrorResourceConflict, false, "target management relation belongs to a different tenant or kind")
		case record.FencingGeneration > execution.FencingGeneration:
			return Result{}, fail(409, ErrorFenced, false, "target fencing generation %d is newer than request generation %d", record.FencingGeneration, execution.FencingGeneration)
		case record.FencingGeneration == execution.FencingGeneration:
			if record.StepID != execution.StepID || record.OperationID != execution.OperationID ||
				record.IdempotencyKey != execution.IdempotencyKey ||
				record.ExecutionAttemptID != execution.ExecutionAttemptID || record.Action != input.Action {
				return Result{}, fail(409, ErrorResourceConflict, false, "equal-generation lifecycle action is not an exact replay")
			}
			return Result{Outputs: copyMap(record.Outputs), Checkpoint: record.Checkpoint}, nil
		}
	}

	if input.Action == "unmanage" && (!ok || !record.Managed) {
		return Result{}, fail(409, ErrorResourceConflict, false, "target is not managed by this lifecycle provider")
	}

	managed := input.Action != "unmanage"

	var provisionOutputs map[string]string
	switch input.Action {
	case "create":
		// Self-hosted creation over user-supplied SSH nodes: bootstrap a real
		// control-plane/workers, store the kubeconfig, and register the target.
		// Without an SSH key the manager falls back to the legacy
		// "register an existing cluster" semantics.
		if input.SSHSecretRef != nil && m.provisioner != nil {
			outputs, err := m.provision(ctx, execution, input)
			if err != nil {
				return Result{}, err
			}
			provisionOutputs = outputs
		} else if err := m.provisionNamespace(ctx, execution, input); err != nil {
			return Result{}, err
		}
	case "import":
		if err := m.validateConnectivity(ctx, execution, input); err != nil {
			return Result{}, err
		}
	case "upgrade":
		if err := m.updateVersion(ctx, execution, input); err != nil {
			return Result{}, err
		}
	case "node-add":
		if m.provisioner == nil {
			return Result{}, fail(400, ErrorUnsupportedAction, false, "node add is not enabled for this provider build")
		}
		// Node addition inherits cluster identity; it does not change the version.
		input.DesiredVersion = record.DesiredVersion
		outputs, err := m.addNodes(ctx, execution, input, record)
		if err != nil {
			return Result{}, err
		}
		provisionOutputs = outputs
	case "unmanage":
		if err := m.cleanupNamespace(ctx, execution, input); err != nil {
			return Result{}, err
		}
	default:
		return Result{}, fail(400, ErrorUnsupportedAction, false, "unsupported action %q", input.Action)
	}

	outputs := map[string]string{
		"targetId":        input.TargetID,
		"targetKind":      input.TargetKind,
		"action":          input.Action,
		"managed":         fmt.Sprintf("%t", managed),
		"observationKind": m.profile.ObservationKind,
	}
	for key, value := range provisionOutputs {
		outputs[key] = value
	}
	if len(input.Parameters) > 0 {
		if encoded, err := json.Marshal(input.Parameters); err == nil {
			outputs["parameters"] = string(encoded)
		}
	}
	if input.DesiredVersion != "" {
		outputs["desiredVersion"] = input.DesiredVersion
	}
	if input.Action == "upgrade" {
		outputs["upgradeMode"] = "metadata-only"
	}
	checkpoint := fmt.Sprintf("%s:%s:%d", input.TargetKind, input.TargetID, execution.FencingGeneration)

	m.mu.Lock()
	m.targets[input.TargetID] = targetRecord{
		TenantID: execution.TenantID, TargetID: input.TargetID, TargetKind: input.TargetKind,
		Action: input.Action, StepID: execution.StepID, OperationID: execution.OperationID,
		IdempotencyKey: execution.IdempotencyKey, ExecutionAttemptID: execution.ExecutionAttemptID,
		FencingGeneration: execution.FencingGeneration, Managed: managed, DesiredVersion: input.DesiredVersion,
		Outputs: outputs, Checkpoint: checkpoint,
	}
	m.mu.Unlock()

	return Result{Outputs: outputs, Checkpoint: checkpoint}, nil
}

func (m *KubernetesManager) validateFencing(execution ExecutionContext, input LifecycleInput) error {
	if input.FencingGeneration != execution.FencingGeneration {
		return fail(400, ErrorInvalidRequest, false, "inputs.fencingGeneration must match execution fencing_generation")
	}
	if execution.FencingGeneration <= 0 {
		return fail(400, ErrorInvalidRequest, false, "fencing_generation must be positive")
	}
	return nil
}

func (m *KubernetesManager) provision(ctx context.Context, execution ExecutionContext, input LifecycleInput) (map[string]string, error) {
	spec, err := m.provisionSpec(execution, input)
	if err != nil {
		return nil, err
	}
	result, err := m.provisioner.Provision(ctx, spec)
	if err != nil {
		return nil, fail(500, ErrorTargetUnavailable, true, "provision cluster: %v", err)
	}
	outputs := map[string]string{
		"provisioned": "true",
		"nodeCount":   fmt.Sprintf("%d", result.NodeCount),
		"endpoint":    result.ControlPlaneEndpoint,
	}
	if ref := result.KubeconfigRef; ref.Provider != "" {
		encoded, _ := json.Marshal(map[string]string{
			"provider": ref.Provider, "scope": ref.Scope, "name": ref.Name, "version": ref.Version,
		})
		outputs["kubeconfigRef"] = string(encoded)
	}
	// Persist the target record inside the provisioned cluster so a provider
	// restart can recover the endpoint/kubeconfig reference for follow-up
	// actions (node add, upgrade) without re-bootstrapping anything.
	if m.kubeconfigLoader != nil {
		kubeconfig, err := m.kubeconfigLoader(ctx, execution.TenantID, result.KubeconfigRef)
		if err != nil {
			return nil, fail(500, ErrorTargetUnavailable, true, "resolve provisioned kubeconfig: %v", err)
		}
		client, err := m.clientFunc(kubeconfig)
		if err != nil {
			return nil, fail(500, ErrorTargetUnavailable, true, "open provisioned cluster: %v", err)
		}
		extra := map[string]string{kubernetesEndpointAnnotation: result.ControlPlaneEndpoint}
		if refJSON := outputs["kubeconfigRef"]; refJSON != "" {
			extra[kubernetesKubeconfigRefAnnotation] = refJSON
		}
		if err := m.writeTargetRecord(ctx, client, execution, input, extra); err != nil {
			return nil, fail(500, ErrorTargetUnavailable, true, "persist target record: %v", err)
		}
	}
	return outputs, nil
}

// provisionSpec translates the lifecycle input into the provisioner spec,
// parsing the user-supplied SSH node list from parameters.
func (m *KubernetesManager) provisionSpec(execution ExecutionContext, input LifecycleInput) (provision.ProvisionSpec, error) {
	if input.SSHSecretRef == nil {
		return provision.ProvisionSpec{}, fail(400, ErrorInvalidRequest, false, "sshSecretRef is required for provisioning")
	}
	nodes, err := parseNodes(input.Parameters)
	if err != nil {
		return provision.ProvisionSpec{}, fail(400, ErrorInvalidRequest, false, "parameters.nodes: %v", err)
	}
	if len(nodes) == 0 {
		return provision.ProvisionSpec{}, fail(400, ErrorInvalidRequest, false, "parameters.nodes requires at least one node")
	}
	return provision.ProvisionSpec{
		TenantID:             execution.TenantID,
		TargetID:             input.TargetID,
		DisplayName:          input.DisplayName,
		DesiredVersion:       input.DesiredVersion,
		PodCIDR:              parameterString(input.Parameters, "podCidr"),
		ServiceCIDR:          parameterString(input.Parameters, "serviceCidr"),
		ClusterDomain:        parameterString(input.Parameters, "clusterDomain"),
		NetworkPlugin:        parameterString(input.Parameters, "networkPlugin"),
		NetworkVersion:       parameterString(input.Parameters, "networkVersion"),
		ImageRepository:      parameterString(input.Parameters, "imageRepository"),
		SandboxImage:         parameterString(input.Parameters, "sandboxImage"),
		CNIImageRepository:   parameterString(input.Parameters, "cniImageRepository"),
		HelmChartRepository:  parameterString(input.Parameters, "helmChartRepository"),
		AdvertiseAddress:     parameterString(input.Parameters, "advertiseAddress"),
		ControlPlaneEndpoint: parameterString(input.Parameters, "controlPlaneEndpoint"),
		IdempotencyKey:       input.IdempotencyKey,
		Nodes:                nodes,
		SSHCredential: provision.SecretReference{
			Provider: input.SSHSecretRef.Provider,
			Scope:    input.SSHSecretRef.Scope,
			Name:     input.SSHSecretRef.Name,
			Version:  input.SSHSecretRef.Version,
		},
	}, nil
}

func parseNodes(params map[string]any) ([]provision.NodeSpec, error) {
	raw, ok := params["nodes"]
	if !ok || raw == nil {
		return nil, nil
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("must be an array of node objects")
	}
	nodes := make([]provision.NodeSpec, 0, len(list))
	for i, item := range list {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("nodes[%d] must be an object", i)
		}
		node := provision.NodeSpec{
			ID:   stringValue(obj["id"]),
			Host: stringValue(obj["host"]),
			User: stringValue(obj["user"]),
			Role: provision.NodeRole(stringValue(obj["role"])),
		}
		if port, err := int64Value(obj["port"]); err == nil {
			node.Port = int(port)
		}
		if node.Host == "" || node.Role == "" {
			return nil, fmt.Errorf("nodes[%d] requires host and role", i)
		}
		if node.Role != provision.RoleControlPlane && node.Role != provision.RoleWorker {
			return nil, fmt.Errorf("nodes[%d].role must be control-plane or worker", i)
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func parameterString(params map[string]any, key string) string {
	raw, ok := params[key]
	if !ok || raw == nil {
		return ""
	}
	if text, ok := raw.(string); ok {
		return text
	}
	return fmt.Sprintf("%v", raw)
}

// addNodes joins new worker nodes to the provisioned cluster over SSH. The
// control-plane endpoint and desired version come from the durable target
// record (in-memory or namespace-recovered); the caller supplies only node
// identities plus the SSH credential scope.
func (m *KubernetesManager) addNodes(ctx context.Context, execution ExecutionContext, input LifecycleInput, record targetRecord) (map[string]string, error) {
	endpoint := record.Outputs["endpoint"]
	if endpoint == "" {
		return nil, fail(409, ErrorResourceConflict, false,
			"node add requires a toolkit-provisioned cluster with a recorded control-plane endpoint; imported clusters have no SSH bootstrap path")
	}
	desiredVersion := record.DesiredVersion
	if desiredVersion == "" {
		return nil, fail(409, ErrorResourceConflict, false, "node add requires the cluster's desired version to be recorded")
	}
	nodes, err := parseAddNodes(input.Parameters)
	if err != nil {
		return nil, fail(400, ErrorInvalidRequest, false, "parameters.nodes: %v", err)
	}
	result, err := m.provisioner.AddNodes(ctx, provision.AddNodesSpec{
		TenantID:             execution.TenantID,
		TargetID:             input.TargetID,
		ControlPlaneEndpoint: endpoint,
		DesiredVersion:       desiredVersion,
		IdempotencyKey:       input.IdempotencyKey,
		Nodes:                nodes,
		SSHCredential: provision.SecretReference{
			Provider: input.SSHSecretRef.Provider,
			Scope:    input.SSHSecretRef.Scope,
			Name:     input.SSHSecretRef.Name,
			Version:  input.SSHSecretRef.Version,
		},
	})
	if err != nil {
		return nil, fail(500, ErrorTargetUnavailable, true, "add nodes: %v", err)
	}
	// Refresh the durable record (fencing + action) so a later restart sees
	// the node-add operation rather than the original create.
	if err := m.persistAfterNodeAdd(ctx, execution, input, record); err != nil {
		return nil, err
	}
	return map[string]string{
		"endpoint":           result.Endpoint,
		"kubeconfigRef":      record.Outputs["kubeconfigRef"],
		"nodeCount":          fmt.Sprintf("%d", result.Requested),
		"joinedCount":        fmt.Sprintf("%d", result.Joined),
		"alreadyJoinedCount": fmt.Sprintf("%d", result.AlreadyJoined),
	}, nil
}

// parseAddNodes translates the intent-owned node identities (name + worker
// role only) into SSH node specs: the name doubles as the SSH address and the
// kubelet node name. Execution fields cannot arrive through this path.
func parseAddNodes(params map[string]any) ([]provision.NodeSpec, error) {
	raw, ok := params["nodes"]
	if !ok || raw == nil {
		return nil, fmt.Errorf("nodes list is required")
	}
	list, ok := raw.([]any)
	if !ok || len(list) == 0 {
		return nil, fmt.Errorf("must be a non-empty array of node objects")
	}
	nodes := make([]provision.NodeSpec, 0, len(list))
	seen := make(map[string]bool, len(list))
	for i, item := range list {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("nodes[%d] must be an object", i)
		}
		name, _ := obj["name"].(string)
		role, _ := obj["role"].(string)
		if name == "" || role != string(provision.RoleWorker) {
			return nil, fmt.Errorf("nodes[%d] requires a name and the worker role", i)
		}
		if seen[name] {
			return nil, fmt.Errorf("nodes[%d]: duplicate node name %q", i, name)
		}
		seen[name] = true
		nodes = append(nodes, provision.NodeSpec{ID: name, Host: name, Role: provision.RoleWorker})
	}
	return nodes, nil
}

// persistAfterNodeAdd refreshes the managed-namespace record after a
// successful node add. Legacy records without a kubeconfig reference skip the
// bookkeeping rather than failing the completed join.
func (m *KubernetesManager) persistAfterNodeAdd(ctx context.Context, execution ExecutionContext, input LifecycleInput, record targetRecord) error {
	if m.kubeconfigLoader == nil {
		return nil
	}
	refJSON := record.Outputs["kubeconfigRef"]
	if refJSON == "" {
		return nil
	}
	var ref provision.SecretReference
	if err := json.Unmarshal([]byte(refJSON), &ref); err != nil {
		return nil
	}
	kubeconfig, err := m.kubeconfigLoader(ctx, execution.TenantID, ref)
	if err != nil {
		return fail(500, ErrorTargetUnavailable, true, "resolve stored kubeconfig: %v", err)
	}
	client, err := m.clientFunc(kubeconfig)
	if err != nil {
		return fail(500, ErrorTargetUnavailable, true, "open target cluster: %v", err)
	}
	extra := map[string]string{kubernetesKubeconfigRefAnnotation: refJSON}
	if endpoint := record.Outputs["endpoint"]; endpoint != "" {
		extra[kubernetesEndpointAnnotation] = endpoint
	}
	if err := m.writeTargetRecord(ctx, client, execution, input, extra); err != nil {
		return fail(500, ErrorTargetUnavailable, true, "persist target record: %v", err)
	}
	return nil
}

// writeTargetRecord creates or refreshes the managed-namespace record inside
// the target cluster, carrying the fencing/operation identity plus the extra
// annotations (endpoint, kubeconfig reference). Ownership is validated before
// any update so a foreign namespace is never rewritten.
func (m *KubernetesManager) writeTargetRecord(ctx context.Context, client kubernetes.Interface, execution ExecutionContext, input LifecycleInput, extra map[string]string) error {
	namespaceName := "hnb-target-" + input.TargetID[:8]
	annotations := map[string]string{
		kubernetesManagedByAnnotation: kubernetesProviderValue,
		kubernetesTenantAnnotation:    execution.TenantID,
		kubernetesTargetAnnotation:    input.TargetID,
		kubernetesOperationAnnotation: execution.OperationID,
		kubernetesFencingAnnotation:   fmt.Sprintf("%d", execution.FencingGeneration),
		kubernetesStepAnnotation:      execution.StepID,
		kubernetesIdemAnnotation:      execution.IdempotencyKey,
		kubernetesAttemptAnnotation:   execution.ExecutionAttemptID,
		kubernetesActionAnnotation:    input.Action,
	}
	if input.DesiredVersion != "" {
		annotations[kubernetesVersionAnnotation] = input.DesiredVersion
	}
	mergeParameterAnnotations(annotations, input.Parameters)
	for key, value := range extra {
		annotations[key] = value
	}
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:        namespaceName,
			Annotations: annotations,
			Labels: map[string]string{
				"hnb.io/managed-by": kubernetesProviderValue,
				"hnb.io/target-id":  input.TargetID,
			},
		},
	}
	_, err := client.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err == nil {
		return nil
	}
	if !apierrors.IsAlreadyExists(err) {
		return targetError("create managed namespace", err)
	}
	existing, err := client.CoreV1().Namespaces().Get(ctx, namespaceName, metav1.GetOptions{})
	if err != nil {
		return targetError("get managed namespace", err)
	}
	ann := existing.GetAnnotations()
	if ann[kubernetesManagedByAnnotation] != kubernetesProviderValue ||
		ann[kubernetesTenantAnnotation] != execution.TenantID || ann[kubernetesTargetAnnotation] != input.TargetID {
		return fail(409, ErrorResourceConflict, false, "managed namespace belongs to a different target management relation")
	}
	// Follow-up actions keep carrying the durable identity annotations when
	// they do not restate them (for example the version on node add).
	if _, present := annotations[kubernetesVersionAnnotation]; !present && ann[kubernetesVersionAnnotation] != "" {
		annotations[kubernetesVersionAnnotation] = ann[kubernetesVersionAnnotation]
	}
	for _, key := range []string{kubernetesEndpointAnnotation, kubernetesKubeconfigRefAnnotation} {
		if _, present := annotations[key]; !present && ann[key] != "" {
			annotations[key] = ann[key]
		}
	}
	existing.SetAnnotations(annotations)
	if _, err := client.CoreV1().Namespaces().Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		return targetError("update managed namespace", err)
	}
	return nil
}

func stringValue(v any) string {
	if text, ok := v.(string); ok {
		return text
	}
	return ""
}

func int64Value(v any) (int64, error) {
	switch typed := v.(type) {
	case float64:
		return int64(typed), nil
	case int64:
		return typed, nil
	case int:
		return int64(typed), nil
	case string:
		return strconv.ParseInt(typed, 10, 64)
	}
	return 0, fmt.Errorf("not an integer")
}

func (m *KubernetesManager) provisionNamespace(ctx context.Context, execution ExecutionContext, input LifecycleInput) error {
	client, err := m.k8sClient(ctx, execution, input)
	if err != nil {
		return err
	}
	namespaceName := "hnb-target-" + input.TargetID[:8]
	annotations := map[string]string{
		kubernetesManagedByAnnotation: kubernetesProviderValue,
		kubernetesTenantAnnotation:    execution.TenantID,
		kubernetesTargetAnnotation:    input.TargetID,
		kubernetesOperationAnnotation: execution.OperationID,
		kubernetesFencingAnnotation:   fmt.Sprintf("%d", execution.FencingGeneration),
		kubernetesVersionAnnotation:   input.DesiredVersion,
		kubernetesStepAnnotation:      execution.StepID,
		kubernetesIdemAnnotation:      execution.IdempotencyKey,
		kubernetesAttemptAnnotation:   execution.ExecutionAttemptID,
		kubernetesActionAnnotation:    input.Action,
	}
	mergeParameterAnnotations(annotations, input.Parameters)
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:        namespaceName,
			Annotations: annotations,
			Labels: map[string]string{
				"hnb.io/managed-by": kubernetesProviderValue,
				"hnb.io/target-id":  input.TargetID,
			},
		},
	}
	_, err = client.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil {
		return targetError("create managed namespace", err)
	}
	return nil
}

func (m *KubernetesManager) validateConnectivity(ctx context.Context, execution ExecutionContext, input LifecycleInput) error {
	client, err := m.k8sClient(ctx, execution, input)
	if err != nil {
		return err
	}
	_, err = client.Discovery().ServerVersion()
	if err != nil {
		return targetError("validate kubernetes connectivity", err)
	}
	return nil
}

func (m *KubernetesManager) updateVersion(ctx context.Context, execution ExecutionContext, input LifecycleInput) error {
	client, err := m.k8sClient(ctx, execution, input)
	if err != nil {
		return err
	}
	namespaceName := "hnb-target-" + input.TargetID[:8]
	existing, err := client.CoreV1().Namespaces().Get(ctx, namespaceName, metav1.GetOptions{})
	if err != nil {
		return targetError("get managed namespace", err)
	}
	ann := existing.GetAnnotations()
	if ann == nil {
		ann = make(map[string]string)
	}
	if ann[kubernetesManagedByAnnotation] != kubernetesProviderValue {
		return fail(409, ErrorResourceConflict, false, "namespace is not managed by this provider")
	}
	if ann[kubernetesTenantAnnotation] != execution.TenantID {
		return fail(409, ErrorResourceConflict, false, "namespace belongs to a different tenant")
	}
	ann[kubernetesVersionAnnotation] = input.DesiredVersion
	ann[kubernetesFencingAnnotation] = fmt.Sprintf("%d", execution.FencingGeneration)
	ann[kubernetesOperationAnnotation] = execution.OperationID
	ann[kubernetesStepAnnotation] = execution.StepID
	ann[kubernetesIdemAnnotation] = execution.IdempotencyKey
	ann[kubernetesAttemptAnnotation] = execution.ExecutionAttemptID
	ann[kubernetesActionAnnotation] = input.Action
	mergeParameterAnnotations(ann, input.Parameters)
	existing.SetAnnotations(ann)
	_, err = client.CoreV1().Namespaces().Update(ctx, existing, metav1.UpdateOptions{})
	if err != nil {
		return targetError("update managed namespace", err)
	}
	return nil
}

func (m *KubernetesManager) loadRecordFromNamespace(ctx context.Context, execution ExecutionContext, input LifecycleInput) (targetRecord, bool, error) {
	client, err := m.k8sClient(ctx, execution, input)
	if err != nil {
		return targetRecord{}, false, err
	}
	ns, err := client.CoreV1().Namespaces().Get(ctx, "hnb-target-"+input.TargetID[:8], metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return targetRecord{}, false, nil
	}
	if err != nil {
		return targetRecord{}, false, targetError("get managed namespace", err)
	}
	ann := ns.GetAnnotations()
	if ann[kubernetesManagedByAnnotation] != kubernetesProviderValue {
		return targetRecord{}, false, nil
	}
	if ann[kubernetesTenantAnnotation] != execution.TenantID || ann[kubernetesTargetAnnotation] != input.TargetID {
		return targetRecord{}, false, fail(409, ErrorResourceConflict, false, "namespace belongs to a different target management relation")
	}
	var generation int64
	if _, err := fmt.Sscanf(ann[kubernetesFencingAnnotation], "%d", &generation); err != nil || generation <= 0 {
		return targetRecord{}, false, fail(409, ErrorResourceConflict, false, "managed namespace has invalid fencing generation")
	}
	managed := ann[kubernetesActionAnnotation] != "unmanage"
	outputs := map[string]string{
		"targetId":        input.TargetID,
		"targetKind":      input.TargetKind,
		"action":          ann[kubernetesActionAnnotation],
		"managed":         fmt.Sprintf("%t", managed),
		"observationKind": m.profile.ObservationKind,
	}
	if ann[kubernetesVersionAnnotation] != "" {
		outputs["desiredVersion"] = ann[kubernetesVersionAnnotation]
	}
	if ann[kubernetesEndpointAnnotation] != "" {
		outputs["endpoint"] = ann[kubernetesEndpointAnnotation]
	}
	if ann[kubernetesKubeconfigRefAnnotation] != "" {
		outputs["kubeconfigRef"] = ann[kubernetesKubeconfigRefAnnotation]
	}
	return targetRecord{
		TenantID: execution.TenantID, TargetID: input.TargetID, TargetKind: input.TargetKind,
		Action: ann[kubernetesActionAnnotation], StepID: ann[kubernetesStepAnnotation], OperationID: ann[kubernetesOperationAnnotation],
		IdempotencyKey: ann[kubernetesIdemAnnotation], ExecutionAttemptID: ann[kubernetesAttemptAnnotation],
		FencingGeneration: generation, Managed: managed, DesiredVersion: ann[kubernetesVersionAnnotation],
		Outputs: outputs, Checkpoint: fmt.Sprintf("%s:%s:%d", input.TargetKind, input.TargetID, generation),
	}, true, nil
}

func mergeParameterAnnotations(annotations map[string]string, params map[string]any) {
	for key, value := range params {
		name := strings.TrimSpace(key)
		if name == "" || value == nil || len(name) > 50 {
			continue
		}
		annotations["hnb.io/parameter."+name] = fmt.Sprintf("%v", value)
	}
}

func (m *KubernetesManager) cleanupNamespace(ctx context.Context, execution ExecutionContext, input LifecycleInput) error {
	client, err := m.k8sClient(ctx, execution, input)
	if err != nil {
		return err
	}
	namespaceName := "hnb-target-" + input.TargetID[:8]
	existing, err := client.CoreV1().Namespaces().Get(ctx, namespaceName, metav1.GetOptions{})
	if err != nil {
		return targetError("get managed namespace", err)
	}
	ann := existing.GetAnnotations()
	if ann[kubernetesManagedByAnnotation] != kubernetesProviderValue {
		return fail(409, ErrorResourceConflict, false, "namespace is not managed by this provider")
	}
	if ann[kubernetesTenantAnnotation] != execution.TenantID {
		return fail(409, ErrorResourceConflict, false, "namespace belongs to a different tenant")
	}
	err = client.CoreV1().Namespaces().Delete(ctx, namespaceName, metav1.DeleteOptions{})
	if err != nil {
		return targetError("delete managed namespace", err)
	}
	return nil
}

func (m *KubernetesManager) k8sClient(ctx context.Context, execution ExecutionContext, _ LifecycleInput) (kubernetes.Interface, error) {
	kubeconfig, ok := execution.Inputs["_resolvedSecretContent"].(string)
	if !ok || kubeconfig == "" {
		return nil, fail(400, ErrorInvalidRequest, false, "resolved kubeconfig is required for kubernetes lifecycle actions")
	}
	if m.client != nil && m.cachedKube == kubeconfig {
		return m.client, nil
	}
	client, err := m.clientFunc([]byte(kubeconfig))
	if err != nil {
		return nil, fail(400, ErrorInvalidRequest, false, "invalid kubeconfig: %v", err)
	}
	m.client = client
	m.cachedKube = kubeconfig
	return client, nil
}

func (m *KubernetesManager) HealthCheck(ctx context.Context) error {
	return nil
}
