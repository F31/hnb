package provider

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	managedByAnnotation          = "hnb.io/managed-by"
	tenantAnnotation             = "hnb.io/tenant-id"
	operationAnnotation          = "hnb.io/operation-id"
	stepAnnotation               = "hnb.io/step-id"
	idempotencyAnnotation        = "hnb.io/idempotency-key"
	fencingGenerationAnnotation  = "hnb.io/fencing-generation"
	executionAttemptIDAnnotation = "hnb.io/execution-attempt-id"
	lastActionAnnotation         = "hnb.io/last-action"
	managedByValue               = "hnb-edge-provider"
	maxCASAttempts               = 5
)

const ()

var edgeAppGVR = schema.GroupVersionResource{
	Group:    "apps.kubeedge.io",
	Version:  "v1alpha1",
	Resource: "edgeapplications",
}

type ExecutionContext struct {
	StepID                  string
	OperationID             string
	TenantID                string
	ProjectID               string
	EnvironmentID           string
	StepType                string
	Inputs                  map[string]any
	ProviderID              string
	ProviderVersion         string
	ProviderDigest          string
	ProviderProtocolVersion string
	Checkpoint              string
	IdempotencyKey          string
	ExecutionAttemptID      string
	FencingGeneration       int64
}

type Result struct {
	Outputs    map[string]any
	Checkpoint string
}

type ErrorCode string

const (
	ErrorInvalidRequest    ErrorCode = "INVALID_REQUEST"
	ErrorScopeDenied       ErrorCode = "SCOPE_DENIED"
	ErrorUnsupportedAction ErrorCode = "UNSUPPORTED_ACTION"
	ErrorResourceConflict  ErrorCode = "RESOURCE_CONFLICT"
	ErrorFenced            ErrorCode = "FENCED"
	ErrorTargetUnavailable ErrorCode = "TARGET_UNAVAILABLE"
	ErrorCancelled         ErrorCode = "CANCELLED"
	ErrorInternal          ErrorCode = "INTERNAL"
)

type StatusError struct {
	HTTPCode  int
	ErrorCode ErrorCode
	Retryable bool
	Err       error
}

func (e *StatusError) Error() string { return e.Err.Error() }

func fail(httpCode int, errorCode ErrorCode, retryable bool, format string, args ...any) error {
	return &StatusError{HTTPCode: httpCode, ErrorCode: errorCode, Retryable: retryable, Err: fmt.Errorf(format, args...)}
}

func invalid(format string, args ...any) error {
	return fail(400, ErrorInvalidRequest, false, format, args...)
}

func conflict(format string, args ...any) error {
	return fail(409, ErrorResourceConflict, false, format, args...)
}

func fenced(stored, requested int64) error {
	return fail(409, ErrorFenced, false, "edge application fencing generation %d is newer than request generation %d", stored, requested)
}

func targetError(operation string, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return fail(408, ErrorCancelled, true, "%s: %v", operation, err)
	}
	return fail(503, ErrorTargetUnavailable, true, "%s: %v", operation, err)
}

type Executor struct {
	k8sClient     kubernetes.Interface
	dynamicClient dynamic.Interface
	restMapper    meta.RESTMapper
	allowed       map[string]struct{}
	maxReplicas   int32
	pollInterval  time.Duration
}

func NewExecutor(k8sClient kubernetes.Interface, dynamicClient dynamic.Interface, restMapper meta.RESTMapper, allowed map[string]struct{}, maxReplicas int32) *Executor {
	return &Executor{
		k8sClient:     k8sClient,
		dynamicClient: dynamicClient,
		restMapper:    restMapper,
		allowed:       allowed,
		maxReplicas:   maxReplicas,
		pollInterval:  200 * time.Millisecond,
	}
}

func NewCloudCoreClient(endpoint, kubeconfigPath string) (kubernetes.Interface, dynamic.Interface, meta.RESTMapper, error) {
	var config *rest.Config
	var err error

	if kubeconfigPath != "" {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	} else {
		config, err = rest.InClusterConfig()
	}
	if err != nil {
		return nil, nil, nil, fmt.Errorf("kubernetes config: %w", err)
	}

	config.Host = endpoint
	config.Timeout = 30 * time.Second

	k8sClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("kubernetes client: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("dynamic client: %w", err)
	}

	restMapper, err := kubernetesDiscoveryRESTMapper(config)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("rest mapper: %w", err)
	}

	return k8sClient, dynamicClient, restMapper, nil
}

func kubernetesDiscoveryRESTMapper(config *rest.Config) (meta.RESTMapper, error) {
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, err
	}
	apiGroupResources, err := restmapper.GetAPIGroupResources(discoveryClient)
	if err != nil {
		return nil, err
	}
	return restmapper.NewDiscoveryRESTMapper(apiGroupResources), nil
}

func (e *Executor) Execute(ctx context.Context, execution ExecutionContext) (Result, error) {
	if execution.TenantID == "" || execution.OperationID == "" || execution.StepID == "" || execution.IdempotencyKey == "" {
		return Result{}, invalid("tenant, operation, step, and idempotency key are required")
	}
	parsedAttempt, err := uuid.Parse(execution.ExecutionAttemptID)
	if err != nil || parsedAttempt == uuid.Nil || parsedAttempt.String() != execution.ExecutionAttemptID {
		return Result{}, invalid("execution_attempt_id must be a canonical UUID")
	}
	if execution.FencingGeneration <= 0 {
		return Result{}, invalid("fencing_generation must be positive")
	}
	namespace, name, err := e.validateResource(stringInputs(execution.Inputs))
	if err != nil {
		return Result{}, err
	}
	stringInput := stringInputs(execution.Inputs)
	switch execution.StepType {
	case "deploy":
		if err := validateInputKeys(stringInput, "namespace", "name", "image", "replicas", "expected_uid", "node_group"); err != nil {
			return Result{}, err
		}
		return e.deploy(ctx, execution, namespace, name)
	case "delete":
		if err := validateInputKeys(stringInput, "namespace", "name", "expected_uid"); err != nil {
			return Result{}, err
		}
		return e.delete(ctx, execution, namespace, name)
	default:
		return Result{}, fail(400, ErrorUnsupportedAction, false, "unsupported step type %q", execution.StepType)
	}
}

func validateInputKeys(inputs map[string]string, allowed ...string) error {
	valid := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		valid[key] = struct{}{}
	}
	for key := range inputs {
		if _, ok := valid[key]; !ok {
			return invalid("unsupported input %q", key)
		}
	}
	return nil
}

func stringInputs(inputs map[string]any) map[string]string {
	if len(inputs) == 0 {
		return nil
	}
	out := make(map[string]string, len(inputs))
	for k, v := range inputs {
		out[k] = fmt.Sprintf("%v", v)
	}
	return out
}

func (e *Executor) validateResource(inputs map[string]string) (string, string, error) {
	namespace, name := inputs["namespace"], inputs["name"]
	if _, ok := e.allowed[namespace]; !ok {
		return "", "", fail(403, ErrorScopeDenied, false, "namespace %q is not allowed", namespace)
	}
	if issues := validation.IsDNS1123Label(namespace); len(issues) != 0 {
		return "", "", invalid("invalid namespace")
	}
	if issues := validation.IsDNS1123Subdomain(name); len(issues) != 0 {
		return "", "", invalid("invalid edge application name")
	}
	return namespace, name, nil
}

func (e *Executor) deploy(ctx context.Context, execution ExecutionContext, namespace, name string) (Result, error) {
	stringInput := stringInputs(execution.Inputs)
	desired, err := e.desiredEdgeApp(execution, namespace, name)
	if err != nil {
		return Result{}, err
	}
	client := e.edgeAppClient(namespace)

	for attempt := 0; attempt < maxCASAttempts; attempt++ {
		existing, err := client.Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return Result{}, targetError("get edge application", err)
			}
			if _, err = client.Create(ctx, desired.DeepCopy(), metav1.CreateOptions{}); err == nil {
				return e.waitAvailable(ctx, execution, namespace, name)
			}
			if apierrors.IsAlreadyExists(err) {
				continue
			}
			return Result{}, targetError("create edge application", err)
		}

		if err := checkManagedTenant(existing, execution); err != nil {
			return Result{}, err
		}
		storedGeneration, err := storedFencingGeneration(existing)
		if err != nil {
			return Result{}, err
		}
		if storedGeneration > execution.FencingGeneration {
			return Result{}, fenced(storedGeneration, execution.FencingGeneration)
		}
		if storedGeneration == execution.FencingGeneration {
			if !exactExecution(existing, execution, "deploy") || !sameEdgeAppSpec(existing, desired) {
				return Result{}, conflict("equal-generation deploy is not an exact replay")
			}
			return e.waitAvailable(ctx, execution, namespace, name)
		}
		if action, _ := getAnnotation(existing, lastActionAnnotation); action == "delete" {
			if stringInput["expected_uid"] == "" || stringInput["expected_uid"] != string(existing.GetUID()) {
				return Result{}, conflict("tombstone redeploy requires matching expected_uid")
			}
			if !sameEdgeAppTemplate(existing, desired) {
				return Result{}, conflict("tombstone edge application specification conflicts with redeploy")
			}
			updated := existing.DeepCopy()
			if err := unstructured.SetNestedField(updated.Object, desired.Object["spec"], "spec", "replicas"); err != nil {
				return Result{}, targetError("set replicas", err)
			}
			setExecutionAnnotations(updated, execution, "deploy")
			if _, err := client.Update(ctx, updated, metav1.UpdateOptions{}); err == nil {
				return e.waitAvailable(ctx, execution, namespace, name)
			} else if apierrors.IsConflict(err) || apierrors.IsNotFound(err) {
				continue
			} else {
				return Result{}, targetError("redeploy tombstone", err)
			}
		}
		if action, _ := getAnnotation(existing, lastActionAnnotation); action != "deploy" || !sameLogicalIdentity(existing, execution) || !sameEdgeAppSpec(existing, desired) {
			return Result{}, conflict("edge application logical identity or specification conflicts with takeover")
		}

		updated := existing.DeepCopy()
		setExecutionAnnotations(updated, execution, "deploy")
		if _, err := client.Update(ctx, updated, metav1.UpdateOptions{}); err == nil {
			return e.waitAvailable(ctx, execution, namespace, name)
		} else if apierrors.IsConflict(err) || apierrors.IsNotFound(err) {
			continue
		} else {
			return Result{}, targetError("take over edge application", err)
		}
	}
	return Result{}, fail(503, ErrorTargetUnavailable, true, "edge application changed during bounded CAS retries")
}

func (e *Executor) desiredEdgeApp(execution ExecutionContext, namespace, name string) (*unstructured.Unstructured, error) {
	stringInput := stringInputs(execution.Inputs)
	image := stringInput["image"]
	if image == "" || len(image) > 512 || strings.ContainsAny(image, " \t\r\n") {
		return nil, invalid("invalid image")
	}
	replicas := int64(1)
	if value := stringInput["replicas"]; value != "" {
		parsed, err := strconv.ParseInt(value, 10, 32)
		if err != nil || strconv.FormatInt(parsed, 10) != value {
			return nil, invalid("replicas must be a canonical integer")
		}
		replicas = parsed
	}
	if replicas < 1 || replicas > int64(e.maxReplicas) {
		return nil, invalid("replicas must be between 1 and %d", e.maxReplicas)
	}

	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion("apps.kubeedge.io/v1alpha1")
	obj.SetKind("EdgeApplication")
	obj.SetName(name)
	obj.SetNamespace(namespace)
	obj.SetLabels(map[string]string{
		"app.kubernetes.io/name":       name,
		"app.kubernetes.io/managed-by": managedByValue,
	})

	if err := unstructured.SetNestedField(obj.Object, replicas, "spec", "replicas"); err != nil {
		return nil, fmt.Errorf("set replicas: %w", err)
	}

	workload := map[string]any{
		"template": map[string]any{
			"spec": map[string]any{
				"containers": []any{
					map[string]any{
						"name":  "workload",
						"image": image,
					},
				},
			},
		},
	}
	if err := unstructured.SetNestedField(obj.Object, workload, "spec", "workloadTemplate"); err != nil {
		return nil, fmt.Errorf("set workload: %w", err)
	}

	if ng := stringInput["node_group"]; ng != "" {
		nodeSelector := map[string]any{
			"matchLabels": map[string]any{
				"hnb.io/node-group": ng,
			},
		}
		if err := unstructured.SetNestedField(obj.Object, nodeSelector, "spec", "nodeSelector"); err != nil {
			return nil, fmt.Errorf("set nodeSelector: %w", err)
		}
	}

	setExecutionAnnotations(obj, execution, "deploy")
	return obj, nil
}

func (e *Executor) delete(ctx context.Context, execution ExecutionContext, namespace, name string) (Result, error) {
	expectedUID := stringInputs(execution.Inputs)["expected_uid"]
	if expectedUID == "" {
		return Result{}, invalid("expected_uid is required")
	}
	client := e.edgeAppClient(namespace)

	for attempt := 0; attempt < maxCASAttempts; attempt++ {
		existing, err := client.Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return Result{}, conflict("edge application does not exist; expected_uid cannot be verified")
		}
		if err != nil {
			return Result{}, targetError("get edge application", err)
		}
		if err := checkManagedTenant(existing, execution); err != nil {
			return Result{}, err
		}
		if string(existing.GetUID()) != expectedUID {
			return Result{}, conflict("expected_uid does not match edge application UID")
		}
		storedGeneration, err := storedFencingGeneration(existing)
		if err != nil {
			return Result{}, err
		}
		if storedGeneration > execution.FencingGeneration {
			return Result{}, fenced(storedGeneration, execution.FencingGeneration)
		}
		if storedGeneration == execution.FencingGeneration {
			if !exactExecution(existing, execution, "delete") {
				return Result{}, conflict("equal-generation delete is not an exact replay")
			}
			replicas, _, _ := unstructured.NestedInt64(existing.Object, "spec", "replicas")
			if replicas != 0 {
				return Result{}, conflict("equal-generation delete is not an exact replay")
			}
			return e.waitDeleted(ctx, execution, namespace, name)
		}
		action, _ := getAnnotation(existing, lastActionAnnotation)
		if action != "deploy" && action != "delete" {
			return Result{}, conflict("edge application has an invalid last action")
		}
		if action == "delete" && !sameLogicalIdentity(existing, execution) {
			return Result{}, conflict("delete tombstone logical identity conflicts with takeover")
		}

		updated := existing.DeepCopy()
		if err := unstructured.SetNestedField(updated.Object, int64(0), "spec", "replicas"); err != nil {
			return Result{}, targetError("set replicas to zero", err)
		}
		setExecutionAnnotations(updated, execution, "delete")
		if _, err := client.Update(ctx, updated, metav1.UpdateOptions{}); err == nil {
			return e.waitDeleted(ctx, execution, namespace, name)
		} else if apierrors.IsConflict(err) || apierrors.IsNotFound(err) {
			continue
		} else {
			return Result{}, targetError("write edge application tombstone", err)
		}
	}
	return Result{}, fail(503, ErrorTargetUnavailable, true, "edge application changed during bounded CAS retries")
}

func (e *Executor) waitAvailable(ctx context.Context, execution ExecutionContext, namespace, name string) (Result, error) {
	client := e.edgeAppClient(namespace)
	var edgeApp *unstructured.Unstructured
	err := wait.PollUntilContextCancel(ctx, e.pollInterval, true, func(ctx context.Context) (bool, error) {
		current, err := client.Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		storedGeneration, err := storedFencingGeneration(current)
		if err != nil {
			return false, err
		}
		if storedGeneration > execution.FencingGeneration {
			return false, fenced(storedGeneration, execution.FencingGeneration)
		}
		if storedGeneration != execution.FencingGeneration || !exactExecution(current, execution, "deploy") {
			return false, conflict("edge application changed while waiting for availability")
		}
		edgeApp = current
		currentReplicas, _, _ := unstructured.NestedInt64(current.Object, "spec", "replicas")
		statusReplicas, _, _ := unstructured.NestedInt64(current.Object, "status", "readyReplicas")
		return currentReplicas > 0 && statusReplicas >= currentReplicas, nil
	})
	if err != nil {
		var statusErr *StatusError
		if errors.As(err, &statusErr) {
			return Result{}, statusErr
		}
		return Result{}, targetError("wait for edge application availability", err)
	}
	return edgeAppResult(edgeApp, "deployed"), nil
}

func (e *Executor) waitDeleted(ctx context.Context, execution ExecutionContext, namespace, name string) (Result, error) {
	client := e.edgeAppClient(namespace)
	var edgeApp *unstructured.Unstructured
	err := wait.PollUntilContextCancel(ctx, e.pollInterval, true, func(ctx context.Context) (bool, error) {
		current, err := client.Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		storedGeneration, err := storedFencingGeneration(current)
		if err != nil {
			return false, err
		}
		if storedGeneration > execution.FencingGeneration {
			return false, fenced(storedGeneration, execution.FencingGeneration)
		}
		if storedGeneration != execution.FencingGeneration || !exactExecution(current, execution, "delete") {
			return false, conflict("edge application changed while waiting for logical deletion")
		}
		edgeApp = current
		replicas, _, _ := unstructured.NestedInt64(current.Object, "spec", "replicas")
		statusReplicas, _, _ := unstructured.NestedInt64(current.Object, "status", "readyReplicas")
		return replicas == 0 && statusReplicas == 0, nil
	})
	if err != nil {
		var statusErr *StatusError
		if errors.As(err, &statusErr) {
			return Result{}, statusErr
		}
		return Result{}, targetError("wait for logical deletion", err)
	}
	return edgeAppResult(edgeApp, "deleted"), nil
}

func (e *Executor) HealthCheck(ctx context.Context) error {
	_, err := e.k8sClient.Discovery().ServerVersion()
	return err
}

func (e *Executor) CloudCoreVersion(ctx context.Context) (string, error) {
	info, err := e.k8sClient.Discovery().ServerVersion()
	if err != nil {
		return "", err
	}
	return info.GitVersion, nil
}

func (e *Executor) edgeAppClient(namespace string) dynamic.ResourceInterface {
	return e.dynamicClient.Resource(edgeAppGVR).Namespace(namespace)
}

func statusDetails(err error) (int, ErrorCode, bool) {
	var target *StatusError
	if errors.As(err, &target) {
		return target.HTTPCode, target.ErrorCode, target.Retryable
	}
	return 500, ErrorInternal, false
}

func compareVersions(a, b string) int {
	parse := func(v string) []int {
		var parts []int
		for _, s := range strings.Split(strings.TrimPrefix(v, "v"), ".") {
			var n int
			fmt.Sscanf(s, "%d", &n)
			parts = append(parts, n)
		}
		for len(parts) < 3 {
			parts = append(parts, 0)
		}
		return parts
	}
	va, vb := parse(a), parse(b)
	for i := 0; i < 3; i++ {
		if va[i] != vb[i] {
			return va[i] - vb[i]
		}
	}
	return 0
}
func checkManagedTenant(existing *unstructured.Unstructured, execution ExecutionContext) error {
	managedBy, _ := getAnnotation(existing, managedByAnnotation)
	tenantID, _ := getAnnotation(existing, tenantAnnotation)
	if managedBy != managedByValue || tenantID != execution.TenantID {
		return conflict("edge application is not owned by this tenant/provider")
	}
	return nil
}

func storedFencingGeneration(existing *unstructured.Unstructured) (int64, error) {
	value, ok := getAnnotation(existing, fencingGenerationAnnotation)
	if !ok {
		return 0, conflict("edge application has no fencing generation annotation")
	}
	generation, err := strconv.ParseInt(value, 10, 64)
	if err != nil || generation <= 0 || strconv.FormatInt(generation, 10) != value {
		return 0, conflict("edge application has an invalid fencing generation")
	}
	return generation, nil
}

func sameLogicalIdentity(existing *unstructured.Unstructured, execution ExecutionContext) bool {
	tid, _ := getAnnotation(existing, tenantAnnotation)
	oid, _ := getAnnotation(existing, operationAnnotation)
	sid, _ := getAnnotation(existing, stepAnnotation)
	ik, _ := getAnnotation(existing, idempotencyAnnotation)
	return tid == execution.TenantID && oid == execution.OperationID && sid == execution.StepID && ik == execution.IdempotencyKey
}

func exactExecution(existing *unstructured.Unstructured, execution ExecutionContext, action string) bool {
	aid, _ := getAnnotation(existing, executionAttemptIDAnnotation)
	la, _ := getAnnotation(existing, lastActionAnnotation)
	return sameLogicalIdentity(existing, execution) && aid == execution.ExecutionAttemptID && la == action
}

func sameEdgeAppSpec(existing, desired *unstructured.Unstructured) bool {
	existingReplicas, _, _ := unstructured.NestedInt64(existing.Object, "spec", "replicas")
	desiredReplicas, _, _ := unstructured.NestedInt64(desired.Object, "spec", "replicas")
	if existingReplicas != desiredReplicas {
		return false
	}
	return sameEdgeAppTemplate(existing, desired)
}

func sameEdgeAppTemplate(existing, desired *unstructured.Unstructured) bool {
	existingImage := extractContainerImage(existing)
	desiredImage := extractContainerImage(desired)
	return existingImage == desiredImage
}

func extractContainerImage(obj *unstructured.Unstructured) string {
	containers, found, err := unstructured.NestedSlice(obj.Object, "spec", "workloadTemplate", "template", "spec", "containers")
	if err != nil || !found || len(containers) == 0 {
		return ""
	}
	container, ok := containers[0].(map[string]any)
	if !ok {
		return ""
	}
	image, _ := container["image"].(string)
	return image
}

func setExecutionAnnotations(obj *unstructured.Unstructured, execution ExecutionContext, action string) {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[managedByAnnotation] = managedByValue
	annotations[tenantAnnotation] = execution.TenantID
	annotations[operationAnnotation] = execution.OperationID
	annotations[stepAnnotation] = execution.StepID
	annotations[idempotencyAnnotation] = execution.IdempotencyKey
	annotations[fencingGenerationAnnotation] = strconv.FormatInt(execution.FencingGeneration, 10)
	annotations[executionAttemptIDAnnotation] = execution.ExecutionAttemptID
	annotations[lastActionAnnotation] = action
	obj.SetAnnotations(annotations)
}

func getAnnotation(obj *unstructured.Unstructured, key string) (string, bool) {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return "", false
	}
	value, ok := annotations[key]
	return value, ok
}

func edgeAppResult(edgeApp *unstructured.Unstructured, action string) Result {
	return Result{
		Outputs: map[string]any{
			"namespace":       edgeApp.GetNamespace(),
			"name":            edgeApp.GetName(),
			"uid":             string(edgeApp.GetUID()),
			"resourceVersion": edgeApp.GetResourceVersion(),
			"action":          action,
		},
		Checkpoint: fmt.Sprintf("edgeapplication:%s/%s:%s", edgeApp.GetNamespace(), edgeApp.GetName(), edgeApp.GetUID()),
	}
}
