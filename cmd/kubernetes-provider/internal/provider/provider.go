package provider

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
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
	managedByValue               = "hnb-kubernetes-provider"
	maxCASAttempts               = 5
)

const ()

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
	return fail(409, ErrorFenced, false, "deployment fencing generation %d is newer than request generation %d", stored, requested)
}

func targetError(operation string, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return fail(408, ErrorCancelled, true, "%s: %v", operation, err)
	}
	return fail(503, ErrorTargetUnavailable, true, "%s: %v", operation, err)
}

type Executor struct {
	client       kubernetes.Interface
	allowed      map[string]struct{}
	maxReplicas  int32
	pollInterval time.Duration
}

func NewExecutor(client kubernetes.Interface, allowed map[string]struct{}, maxReplicas int32) *Executor {
	return &Executor{client: client, allowed: allowed, maxReplicas: maxReplicas, pollInterval: 200 * time.Millisecond}
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
	switch execution.StepType {
	case "deploy":
		if err := validateInputKeys(stringInputs(execution.Inputs), "namespace", "name", "image", "replicas", "expected_uid"); err != nil {
			return Result{}, err
		}
		return e.deploy(ctx, execution, namespace, name)
	case "delete":
		if err := validateInputKeys(stringInputs(execution.Inputs), "namespace", "name", "expected_uid"); err != nil {
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
		return "", "", invalid("invalid deployment name")
	}
	return namespace, name, nil
}

func (e *Executor) deploy(ctx context.Context, execution ExecutionContext, namespace, name string) (Result, error) {
	desired, err := e.desiredDeployment(execution, namespace, name)
	if err != nil {
		return Result{}, err
	}
	deployments := e.client.AppsV1().Deployments(namespace)
	stringInput := stringInputs(execution.Inputs)

	for attempt := 0; attempt < maxCASAttempts; attempt++ {
		existing, err := deployments.Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			if _, err = deployments.Create(ctx, desired.DeepCopy(), metav1.CreateOptions{}); err == nil {
				return e.waitAvailable(ctx, execution, namespace, name)
			}
			if apierrors.IsAlreadyExists(err) || apierrors.IsConflict(err) {
				continue
			}
			return Result{}, targetError("create deployment", err)
		}
		if err != nil {
			return Result{}, targetError("get deployment", err)
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
			if !exactExecution(existing, execution, "deploy") || !sameDeploySpec(existing, desired) {
				return Result{}, conflict("equal-generation deploy is not an exact replay")
			}
			return e.waitAvailable(ctx, execution, namespace, name)
		}
		if existing.Annotations[lastActionAnnotation] == "delete" {
			if stringInput["expected_uid"] == "" || stringInput["expected_uid"] != string(existing.UID) {
				return Result{}, conflict("tombstone redeploy requires matching expected_uid")
			}
			if !sameDeployTemplate(existing, desired) {
				return Result{}, conflict("tombstone deployment specification conflicts with redeploy")
			}
			updated := existing.DeepCopy()
			updated.Spec.Replicas = desired.Spec.Replicas
			setExecutionAnnotations(updated, execution, "deploy")
			if _, err := deployments.Update(ctx, updated, metav1.UpdateOptions{}); err == nil {
				return e.waitAvailable(ctx, execution, namespace, name)
			} else if apierrors.IsConflict(err) || apierrors.IsNotFound(err) {
				continue
			} else {
				return Result{}, targetError("redeploy tombstone", err)
			}
		}
		if existing.Annotations[lastActionAnnotation] != "deploy" || !sameLogicalIdentity(existing, execution) || !sameDeploySpec(existing, desired) {
			return Result{}, conflict("deployment logical identity or specification conflicts with takeover")
		}

		updated := existing.DeepCopy()
		setExecutionAnnotations(updated, execution, "deploy")
		if _, err := deployments.Update(ctx, updated, metav1.UpdateOptions{}); err == nil {
			return e.waitAvailable(ctx, execution, namespace, name)
		} else if apierrors.IsConflict(err) || apierrors.IsNotFound(err) {
			continue
		} else {
			return Result{}, targetError("take over deployment", err)
		}
	}
	return Result{}, fail(503, ErrorTargetUnavailable, true, "deployment changed during bounded CAS retries")
}

func (e *Executor) desiredDeployment(execution ExecutionContext, namespace, name string) (*appsv1.Deployment, error) {
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
	labels := map[string]string{"app.kubernetes.io/name": name, "app.kubernetes.io/managed-by": managedByValue}
	selector := map[string]string{"hnb.io/workload": name}
	count := int32(replicas)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels},
		Spec: appsv1.DeploymentSpec{
			Replicas: &count,
			Selector: &metav1.LabelSelector{MatchLabels: selector},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: selector},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "workload", Image: image}}},
			},
		},
	}
	setExecutionAnnotations(deployment, execution, "deploy")
	return deployment, nil
}

func checkManagedTenant(deployment *appsv1.Deployment, execution ExecutionContext) error {
	a := deployment.Annotations
	if a[managedByAnnotation] != managedByValue || a[tenantAnnotation] != execution.TenantID {
		return conflict("deployment is not owned by this tenant/provider")
	}
	return nil
}

func storedFencingGeneration(deployment *appsv1.Deployment) (int64, error) {
	value := deployment.Annotations[fencingGenerationAnnotation]
	generation, err := strconv.ParseInt(value, 10, 64)
	if err != nil || generation <= 0 || strconv.FormatInt(generation, 10) != value {
		return 0, conflict("deployment has an invalid fencing generation")
	}
	return generation, nil
}

func sameLogicalIdentity(deployment *appsv1.Deployment, execution ExecutionContext) bool {
	a := deployment.Annotations
	return a[tenantAnnotation] == execution.TenantID &&
		a[operationAnnotation] == execution.OperationID &&
		a[stepAnnotation] == execution.StepID &&
		a[idempotencyAnnotation] == execution.IdempotencyKey
}

func exactExecution(deployment *appsv1.Deployment, execution ExecutionContext, action string) bool {
	return sameLogicalIdentity(deployment, execution) &&
		deployment.Annotations[executionAttemptIDAnnotation] == execution.ExecutionAttemptID &&
		deployment.Annotations[lastActionAnnotation] == action
}

func sameDeploySpec(existing, desired *appsv1.Deployment) bool {
	if existing.Spec.Replicas == nil || desired.Spec.Replicas == nil || *existing.Spec.Replicas != *desired.Spec.Replicas {
		return false
	}
	return sameDeployTemplate(existing, desired)
}

func sameDeployTemplate(existing, desired *appsv1.Deployment) bool {
	if !reflect.DeepEqual(existing.Spec.Selector, desired.Spec.Selector) || !reflect.DeepEqual(existing.Spec.Template.Labels, desired.Spec.Template.Labels) {
		return false
	}
	return len(existing.Spec.Template.Spec.Containers) == 1 &&
		existing.Spec.Template.Spec.Containers[0].Name == "workload" &&
		existing.Spec.Template.Spec.Containers[0].Image == desired.Spec.Template.Spec.Containers[0].Image
}

func setExecutionAnnotations(deployment *appsv1.Deployment, execution ExecutionContext, action string) {
	if deployment.Annotations == nil {
		deployment.Annotations = make(map[string]string)
	}
	deployment.Annotations[managedByAnnotation] = managedByValue
	deployment.Annotations[tenantAnnotation] = execution.TenantID
	deployment.Annotations[operationAnnotation] = execution.OperationID
	deployment.Annotations[stepAnnotation] = execution.StepID
	deployment.Annotations[idempotencyAnnotation] = execution.IdempotencyKey
	deployment.Annotations[fencingGenerationAnnotation] = strconv.FormatInt(execution.FencingGeneration, 10)
	deployment.Annotations[executionAttemptIDAnnotation] = execution.ExecutionAttemptID
	deployment.Annotations[lastActionAnnotation] = action
}

func (e *Executor) waitAvailable(ctx context.Context, execution ExecutionContext, namespace, name string) (Result, error) {
	var deployment *appsv1.Deployment
	err := wait.PollUntilContextCancel(ctx, e.pollInterval, true, func(ctx context.Context) (bool, error) {
		current, err := e.client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
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
			return false, conflict("deployment changed while waiting for availability")
		}
		deployment = current
		desired := int32(1)
		if current.Spec.Replicas != nil {
			desired = *current.Spec.Replicas
		}
		return current.Status.ObservedGeneration >= current.Generation && current.Status.AvailableReplicas >= desired, nil
	})
	if err != nil {
		var statusErr *StatusError
		if errors.As(err, &statusErr) {
			return Result{}, statusErr
		}
		return Result{}, targetError("wait for deployment availability", err)
	}
	return resourceResult(deployment, "deployed"), nil
}

func (e *Executor) delete(ctx context.Context, execution ExecutionContext, namespace, name string) (Result, error) {
	expectedUID := stringInputs(execution.Inputs)["expected_uid"]
	if expectedUID == "" {
		return Result{}, invalid("expected_uid is required")
	}
	deployments := e.client.AppsV1().Deployments(namespace)

	for attempt := 0; attempt < maxCASAttempts; attempt++ {
		deployment, err := deployments.Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return Result{}, conflict("deployment does not exist; expected_uid cannot be verified")
		}
		if err != nil {
			return Result{}, targetError("get deployment", err)
		}
		if err := checkManagedTenant(deployment, execution); err != nil {
			return Result{}, err
		}
		if string(deployment.UID) != expectedUID {
			return Result{}, conflict("expected_uid does not match deployment UID")
		}
		storedGeneration, err := storedFencingGeneration(deployment)
		if err != nil {
			return Result{}, err
		}
		if storedGeneration > execution.FencingGeneration {
			return Result{}, fenced(storedGeneration, execution.FencingGeneration)
		}
		if storedGeneration == execution.FencingGeneration {
			if !exactExecution(deployment, execution, "delete") || deployment.Spec.Replicas == nil || *deployment.Spec.Replicas != 0 {
				return Result{}, conflict("equal-generation delete is not an exact replay")
			}
			return e.waitDeleted(ctx, execution, namespace, name)
		}
		if action := deployment.Annotations[lastActionAnnotation]; action != "deploy" && action != "delete" {
			return Result{}, conflict("deployment has an invalid last action")
		} else if action == "delete" && !sameLogicalIdentity(deployment, execution) {
			return Result{}, conflict("delete tombstone logical identity conflicts with takeover")
		}

		updated := deployment.DeepCopy()
		zero := int32(0)
		updated.Spec.Replicas = &zero
		setExecutionAnnotations(updated, execution, "delete")
		if _, err := deployments.Update(ctx, updated, metav1.UpdateOptions{}); err == nil {
			return e.waitDeleted(ctx, execution, namespace, name)
		} else if apierrors.IsConflict(err) || apierrors.IsNotFound(err) {
			continue
		} else {
			return Result{}, targetError("write deployment tombstone", err)
		}
	}
	return Result{}, fail(503, ErrorTargetUnavailable, true, "deployment changed during bounded CAS retries")
}

func (e *Executor) waitDeleted(ctx context.Context, execution ExecutionContext, namespace, name string) (Result, error) {
	var deployment *appsv1.Deployment
	err := wait.PollUntilContextCancel(ctx, e.pollInterval, true, func(ctx context.Context) (bool, error) {
		current, err := e.client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
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
			return false, conflict("deployment changed while waiting for logical deletion")
		}
		deployment = current
		return current.Spec.Replicas != nil && *current.Spec.Replicas == 0 &&
			current.Status.ObservedGeneration >= current.Generation && current.Status.AvailableReplicas == 0, nil
	})
	if err != nil {
		var statusErr *StatusError
		if errors.As(err, &statusErr) {
			return Result{}, statusErr
		}
		return Result{}, targetError("wait for logical deletion", err)
	}
	return resourceResult(deployment, "deleted"), nil
}

func resourceResult(deployment *appsv1.Deployment, action string) Result {
	return Result{Outputs: map[string]any{"namespace": deployment.Namespace, "name": deployment.Name, "uid": string(deployment.UID), "resourceVersion": deployment.ResourceVersion, "action": action}, Checkpoint: fmt.Sprintf("deployment:%s/%s:%s", deployment.Namespace, deployment.Name, deployment.UID)}
}

func statusDetails(err error) (int, ErrorCode, bool) {
	var target *StatusError
	if errors.As(err, &target) {
		return target.HTTPCode, target.ErrorCode, target.Retryable
	}
	return 500, ErrorInternal, false
}
