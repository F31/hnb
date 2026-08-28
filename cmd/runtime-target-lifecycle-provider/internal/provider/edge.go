package provider

import (
	"context"
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	edgeManagedByAnnotation = "hnb.io/managed-by"
	edgeProviderValue       = "hnb-edge-lifecycle-provider"
	edgeTenantAnnotation    = "hnb.io/tenant-id"
	edgeTargetAnnotation    = "hnb.io/target-id"
	edgeOperationAnnotation = "hnb.io/operation-id"
	edgeFencingAnnotation   = "hnb.io/fencing-generation"
	edgeVersionAnnotation   = "hnb.io/desired-version"
	edgeEndpointAnnotation  = "hnb.io/cloudcore-endpoint"
)

// EdgeManager implements LifecycleManager for EdgeRuntimeTarget lifecycle
// actions (5.7/5.8). It manages a namespace per target on CloudCore to track
// lifecycle state, using k8s annotations for fencing and idempotency.
// The observer is registered/unregistered by the HTTP server's handleExecute
// after Apply returns; the manager only validates the target is manageable.
type EdgeManager struct {
	profile    Profile
	mu         sync.Mutex
	targets    map[string]targetRecord
	clientFunc func(kubeconfig []byte) (kubernetes.Interface, error)
	client     kubernetes.Interface
	cachedKube string
	now        func() time.Time
}

// NewEdgeManager creates a manager that connects to CloudCore via the
// resolved kubeconfig from SecretResolver.
func NewEdgeManager(profile Profile) *EdgeManager {
	return &EdgeManager{
		profile:    profile,
		targets:    make(map[string]targetRecord),
		clientFunc: realKubernetesClient,
		now:        time.Now,
	}
}

// NewEdgeManagerWithClient allows injecting a fake client for testing.
func NewEdgeManagerWithClient(profile Profile, clientFunc func(kubeconfig []byte) (kubernetes.Interface, error)) *EdgeManager {
	return &EdgeManager{
		profile:    profile,
		targets:    make(map[string]targetRecord),
		clientFunc: clientFunc,
		now:        time.Now,
	}
}

// Apply implements LifecycleManager with real CloudCore operations.
// For "import" (register): creates a tenant-bound namespace on CloudCore.
// For "upgrade": validates the target and updates the managed version.
// For "unmanage": cleans up the managed namespace (not non-managed resources).
func (m *EdgeManager) Apply(ctx context.Context, execution ExecutionContext, input LifecycleInput) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, targetError("apply edge lifecycle action", err)
	}

	if err := m.validateFencing(execution, input); err != nil {
		return Result{}, err
	}

	m.mu.Lock()
	record, ok := m.targets[input.TargetID]
	m.mu.Unlock()

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
		return Result{}, fail(409, ErrorResourceConflict, false, "target is not managed by this edge lifecycle provider")
	}

	managed := input.Action != "unmanage"

	switch input.Action {
	case "import":
		if err := m.provisionCloudCoreNamespace(ctx, execution, input); err != nil {
			return Result{}, err
		}
	case "upgrade":
		if err := m.updateVersion(ctx, execution, input); err != nil {
			return Result{}, err
		}
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
	if input.DesiredVersion != "" {
		outputs["desiredVersion"] = input.DesiredVersion
	}
	if input.CloudCoreEndpoint != "" {
		outputs["cloudCoreEndpoint"] = input.CloudCoreEndpoint
	}
	checkpoint := fmt.Sprintf("edge:%s:%s:%d", input.TargetKind, input.TargetID, execution.FencingGeneration)

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

func (m *EdgeManager) validateFencing(execution ExecutionContext, input LifecycleInput) error {
	if input.FencingGeneration != execution.FencingGeneration {
		return fail(400, ErrorInvalidRequest, false, "inputs.fencingGeneration must match execution fencing_generation")
	}
	if execution.FencingGeneration <= 0 {
		return fail(400, ErrorInvalidRequest, false, "fencing_generation must be positive")
	}
	return nil
}

func (m *EdgeManager) provisionCloudCoreNamespace(ctx context.Context, execution ExecutionContext, input LifecycleInput) error {
	client, err := m.k8sClient(ctx, execution, input)
	if err != nil {
		return err
	}
	namespaceName := "hnb-edge-" + input.TargetID[:8]
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespaceName,
			Annotations: map[string]string{
				edgeManagedByAnnotation:  edgeProviderValue,
				edgeTenantAnnotation:     execution.TenantID,
				edgeTargetAnnotation:     input.TargetID,
				edgeOperationAnnotation:  execution.OperationID,
				edgeFencingAnnotation:    fmt.Sprintf("%d", execution.FencingGeneration),
				edgeVersionAnnotation:    input.DesiredVersion,
				edgeEndpointAnnotation:   input.CloudCoreEndpoint,
			},
			Labels: map[string]string{
				"hnb.io/managed-by": edgeProviderValue,
				"hnb.io/target-id":  input.TargetID,
			},
		},
	}
	_, err = client.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil {
		return targetError("create edge managed namespace on CloudCore", err)
	}
	return nil
}

func (m *EdgeManager) updateVersion(ctx context.Context, execution ExecutionContext, input LifecycleInput) error {
	client, err := m.k8sClient(ctx, execution, input)
	if err != nil {
		return err
	}
	namespaceName := "hnb-edge-" + input.TargetID[:8]
	existing, err := client.CoreV1().Namespaces().Get(ctx, namespaceName, metav1.GetOptions{})
	if err != nil {
		return targetError("get edge managed namespace", err)
	}
	ann := existing.GetAnnotations()
	if ann == nil {
		ann = make(map[string]string)
	}
	if ann[edgeManagedByAnnotation] != edgeProviderValue {
		return fail(409, ErrorResourceConflict, false, "namespace is not managed by this edge provider")
	}
	if ann[edgeTenantAnnotation] != execution.TenantID {
		return fail(409, ErrorResourceConflict, false, "namespace belongs to a different tenant")
	}
	ann[edgeVersionAnnotation] = input.DesiredVersion
	ann[edgeFencingAnnotation] = fmt.Sprintf("%d", execution.FencingGeneration)
	existing.SetAnnotations(ann)
	_, err = client.CoreV1().Namespaces().Update(ctx, existing, metav1.UpdateOptions{})
	if err != nil {
		return targetError("update edge managed namespace", err)
	}
	return nil
}

func (m *EdgeManager) cleanupNamespace(ctx context.Context, execution ExecutionContext, input LifecycleInput) error {
	client, err := m.k8sClient(ctx, execution, input)
	if err != nil {
		return err
	}
	namespaceName := "hnb-edge-" + input.TargetID[:8]
	existing, err := client.CoreV1().Namespaces().Get(ctx, namespaceName, metav1.GetOptions{})
	if err != nil {
		return targetError("get edge managed namespace", err)
	}
	ann := existing.GetAnnotations()
	if ann[edgeManagedByAnnotation] != edgeProviderValue {
		return fail(409, ErrorResourceConflict, false, "namespace is not managed by this edge provider")
	}
	if ann[edgeTenantAnnotation] != execution.TenantID {
		return fail(409, ErrorResourceConflict, false, "namespace belongs to a different tenant")
	}
	err = client.CoreV1().Namespaces().Delete(ctx, namespaceName, metav1.DeleteOptions{})
	if err != nil {
		return targetError("delete edge managed namespace", err)
	}
	return nil
}

func (m *EdgeManager) k8sClient(ctx context.Context, execution ExecutionContext, _ LifecycleInput) (kubernetes.Interface, error) {
	kubeconfig, ok := execution.Inputs["_resolvedSecretContent"].(string)
	if !ok || kubeconfig == "" {
		return nil, fail(400, ErrorInvalidRequest, false, "resolved kubeconfig is required for edge lifecycle actions")
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

func (m *EdgeManager) HealthCheck(ctx context.Context) error {
	return nil
}