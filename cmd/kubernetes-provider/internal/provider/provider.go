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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
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
	sourceImageAnnotation        = "hnb.io/source-image"
	targetImageAnnotation        = "hnb.io/target-image"
	backupRefAnnotation          = "hnb.io/backup-ref"
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
		if err := validateInputKeys(stringInputs(execution.Inputs), "namespace", "name", "image", "replicas", "expected_uid", "pvc_name", "pvc_mount_path", "secret_name", "secret_mount_path", "secret_reference", "mount_reference"); err != nil {
			return Result{}, err
		}
		return e.deploy(ctx, execution, namespace, name)
	case "delete":
		if err := validateInputKeys(stringInputs(execution.Inputs), "namespace", "name", "expected_uid"); err != nil {
			return Result{}, err
		}
		return e.delete(ctx, execution, namespace, name)
	case "service.deploy":
		if err := validateInputKeys(stringInputs(execution.Inputs), "namespace", "name", "workload", "port"); err != nil {
			return Result{}, err
		}
		return e.deployService(ctx, execution, namespace, name)
	case "pvc.deploy":
		if err := validateInputKeys(stringInputs(execution.Inputs), "namespace", "name", "storage_class", "size"); err != nil {
			return Result{}, err
		}
		return e.deployPVC(ctx, execution, namespace, name)
	case "workload.upgrade":
		if err := validateInputKeys(stringInputs(execution.Inputs), "namespace", "name", "source_image", "target_image", "expected_uid", "data_compatible", "backup_ref"); err != nil {
			return Result{}, err
		}
		return e.upgrade(ctx, execution, namespace, name)
	case "workload.rollback":
		if err := validateInputKeys(stringInputs(execution.Inputs), "namespace", "name", "source_image", "target_image", "expected_uid", "data_irreversible", "backup_ref"); err != nil {
			return Result{}, err
		}
		return e.rollback(ctx, execution, namespace, name)
	case "pvc.delete":
		if err := validateInputKeys(stringInputs(execution.Inputs), "namespace", "name", "expected_uid", "data_cleanup_authorized"); err != nil {
			return Result{}, err
		}
		return e.deletePVC(ctx, execution, namespace, name)
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
	pvcName, pvcPath := stringInput["pvc_name"], stringInput["pvc_mount_path"]
	secretName, secretPath := stringInput["secret_name"], stringInput["secret_mount_path"]
	if secretName == "" {
		secretName = stringInput["secret_reference"]
	}
	if secretPath == "" {
		secretPath = stringInput["mount_reference"]
	}
	if (pvcName == "") != (pvcPath == "") || (secretName == "") != (secretPath == "") {
		return nil, invalid("pvc and secret mounts require both name and mount path")
	}
	if pvcName != "" {
		if issues := validation.IsDNS1123Subdomain(pvcName); len(issues) != 0 || !validMountPath(pvcPath) {
			return nil, invalid("invalid pvc mount")
		}
	}
	if secretName != "" {
		if issues := validation.IsDNS1123Subdomain(secretName); len(issues) != 0 || !validMountPath(secretPath) {
			return nil, invalid("invalid secret mount")
		}
	}
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
	container := corev1.Container{Name: "workload", Image: image}
	var volumes []corev1.Volume
	if pvcName != "" {
		volumes = append(volumes, corev1.Volume{Name: "data", VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: pvcName}}})
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{Name: "data", MountPath: pvcPath})
	}
	if secretName != "" {
		defaultMode := int32(0o644)
		volumes = append(volumes, corev1.Volume{Name: "secret", VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{SecretName: secretName, DefaultMode: &defaultMode}}})
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{Name: "secret", MountPath: secretPath, ReadOnly: true})
	}
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels},
		Spec: appsv1.DeploymentSpec{
			Replicas: &count,
			Selector: &metav1.LabelSelector{MatchLabels: selector},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: selector},
				Spec:       corev1.PodSpec{Containers: []corev1.Container{container}, Volumes: volumes},
			},
		},
	}
	setExecutionAnnotations(deployment, execution, "deploy")
	return deployment, nil
}

func validMountPath(path string) bool {
	return len(path) > 1 && len(path) <= 4096 && path[0] == '/' && !strings.Contains(path, "..") && !strings.ContainsAny(path, "\n\r")
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
	if len(existing.Spec.Template.Spec.Containers) != 1 || existing.Spec.Template.Spec.Containers[0].Name != "workload" ||
		existing.Spec.Template.Spec.Containers[0].Image != desired.Spec.Template.Spec.Containers[0].Image {
		return false
	}
	return reflect.DeepEqual(existing.Spec.Template.Spec.Volumes, desired.Spec.Template.Spec.Volumes) &&
		reflect.DeepEqual(existing.Spec.Template.Spec.Containers[0].VolumeMounts, desired.Spec.Template.Spec.Containers[0].VolumeMounts)
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
	return e.waitAvailableAction(ctx, execution, namespace, name, "deploy", "deployed")
}

func (e *Executor) waitAvailableAction(ctx context.Context, execution ExecutionContext, namespace, name, action, resultAction string) (Result, error) {
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
		if storedGeneration != execution.FencingGeneration || !exactExecution(current, execution, action) {
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
	return resourceResult(deployment, resultAction), nil
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
		if action := deployment.Annotations[lastActionAnnotation]; action != "deploy" && action != "delete" && action != "workload.upgrade" && action != "workload.rollback" {
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

func (e *Executor) deployService(ctx context.Context, execution ExecutionContext, namespace, name string) (Result, error) {
	inputs := stringInputs(execution.Inputs)
	workload := inputs["workload"]
	if issues := validation.IsDNS1123Subdomain(workload); len(issues) != 0 {
		return Result{}, invalid("invalid workload name")
	}
	portValue := inputs["port"]
	port, err := strconv.ParseInt(portValue, 10, 32)
	if err != nil || strconv.FormatInt(port, 10) != portValue || port < 1 || port > 65535 {
		return Result{}, invalid("port must be a canonical integer between 1 and 65535")
	}
	// The service is an association, not a partial create: verify the target
	// workload exists and is owned before writing anything.
	deployment, err := e.client.AppsV1().Deployments(namespace).Get(ctx, workload, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return Result{}, conflict("associated workload is not present")
	}
	if err != nil {
		return Result{}, targetError("get associated workload", err)
	}
	if err := checkManagedTenant(deployment, execution); err != nil {
		return Result{}, err
	}
	if deployment.Annotations[lastActionAnnotation] == "delete" {
		return Result{}, conflict("associated workload is deleted")
	}
	desired := desiredService(execution, namespace, name, workload, int32(port))
	services := e.client.CoreV1().Services(namespace)
	for attempt := 0; attempt < maxCASAttempts; attempt++ {
		existing, err := services.Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			if created, err := services.Create(ctx, desired.DeepCopy(), metav1.CreateOptions{}); err == nil {
				return serviceResult(created), nil
			} else if apierrors.IsAlreadyExists(err) || apierrors.IsConflict(err) {
				continue
			} else {
				return Result{}, targetError("create service", err)
			}
		}
		if err != nil {
			return Result{}, targetError("get service", err)
		}
		if err := checkManagedService(existing, execution); err != nil {
			return Result{}, err
		}
		storedGeneration, err := storedServiceFencingGeneration(existing)
		if err != nil {
			return Result{}, err
		}
		if storedGeneration > execution.FencingGeneration {
			return Result{}, fenced(storedGeneration, execution.FencingGeneration)
		}
		if storedGeneration == execution.FencingGeneration {
			if !exactServiceExecution(existing, execution) || !sameServiceSpec(existing, desired) {
				return Result{}, conflict("equal-generation service deploy is not an exact replay")
			}
			return serviceResult(existing), nil
		}
		if existing.Annotations[lastActionAnnotation] == "delete" {
			return Result{}, conflict("service tombstone blocks redeploy")
		}
		if !sameServiceLogicalIdentity(existing, execution) || !sameServiceSpec(existing, desired) {
			return Result{}, conflict("service logical identity or specification conflicts with takeover")
		}
		updated := existing.DeepCopy()
		setServiceExecutionAnnotations(updated, execution, "service.deploy")
		if _, err := services.Update(ctx, updated, metav1.UpdateOptions{}); err == nil {
			return serviceResult(updated), nil
		} else if apierrors.IsConflict(err) || apierrors.IsNotFound(err) {
			continue
		} else {
			return Result{}, targetError("take over service", err)
		}
	}
	return Result{}, fail(503, ErrorTargetUnavailable, true, "service changed during bounded CAS retries")
}

func desiredService(execution ExecutionContext, namespace, name, workload string, port int32) *corev1.Service {
	labels := map[string]string{"app.kubernetes.io/name": name, "app.kubernetes.io/managed-by": managedByValue}
	selector := map[string]string{"hnb.io/workload": workload}
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: selector,
			Ports: []corev1.ServicePort{{
				Name: "http", Port: port, TargetPort: intstr.FromInt(int(port)), Protocol: corev1.ProtocolTCP,
			}},
		},
	}
	setServiceExecutionAnnotations(service, execution, "service.deploy")
	return service
}

func checkManagedService(service *corev1.Service, execution ExecutionContext) error {
	a := service.Annotations
	if a[managedByAnnotation] != managedByValue || a[tenantAnnotation] != execution.TenantID {
		return conflict("service is not owned by this tenant/provider")
	}
	return nil
}

func storedServiceFencingGeneration(service *corev1.Service) (int64, error) {
	value := service.Annotations[fencingGenerationAnnotation]
	generation, err := strconv.ParseInt(value, 10, 64)
	if err != nil || generation <= 0 || strconv.FormatInt(generation, 10) != value {
		return 0, conflict("service has an invalid fencing generation")
	}
	return generation, nil
}

func sameServiceLogicalIdentity(service *corev1.Service, execution ExecutionContext) bool {
	a := service.Annotations
	return a[tenantAnnotation] == execution.TenantID &&
		a[operationAnnotation] == execution.OperationID &&
		a[stepAnnotation] == execution.StepID &&
		a[idempotencyAnnotation] == execution.IdempotencyKey
}

func exactServiceExecution(service *corev1.Service, execution ExecutionContext) bool {
	return sameServiceLogicalIdentity(service, execution) &&
		service.Annotations[executionAttemptIDAnnotation] == execution.ExecutionAttemptID &&
		service.Annotations[lastActionAnnotation] == "service.deploy"
}

func sameServiceSpec(existing, desired *corev1.Service) bool {
	if existing.Spec.Type != desired.Spec.Type || !reflect.DeepEqual(existing.Spec.Selector, desired.Spec.Selector) || len(existing.Spec.Ports) != len(desired.Spec.Ports) {
		return false
	}
	for i := range desired.Spec.Ports {
		a, b := existing.Spec.Ports[i], desired.Spec.Ports[i]
		if a.Name != b.Name || a.Port != b.Port || a.Protocol != b.Protocol || a.TargetPort != b.TargetPort {
			return false
		}
	}
	return true
}

func setServiceExecutionAnnotations(service *corev1.Service, execution ExecutionContext, action string) {
	if service.Annotations == nil {
		service.Annotations = make(map[string]string)
	}
	service.Annotations[managedByAnnotation] = managedByValue
	service.Annotations[tenantAnnotation] = execution.TenantID
	service.Annotations[operationAnnotation] = execution.OperationID
	service.Annotations[stepAnnotation] = execution.StepID
	service.Annotations[idempotencyAnnotation] = execution.IdempotencyKey
	service.Annotations[fencingGenerationAnnotation] = strconv.FormatInt(execution.FencingGeneration, 10)
	service.Annotations[executionAttemptIDAnnotation] = execution.ExecutionAttemptID
	service.Annotations[lastActionAnnotation] = action
}

func serviceResult(service *corev1.Service) Result {
	return Result{Outputs: map[string]any{
		"namespace": service.Namespace, "name": service.Name, "uid": string(service.UID),
		"resourceVersion": service.ResourceVersion, "action": "service.deploy",
	}, Checkpoint: fmt.Sprintf("service:%s/%s:%s", service.Namespace, service.Name, service.UID)}
}

func (e *Executor) deployPVC(ctx context.Context, execution ExecutionContext, namespace, name string) (Result, error) {
	inputs := stringInputs(execution.Inputs)
	storageClass := inputs["storage_class"]
	if issues := validation.IsDNS1123Subdomain(storageClass); len(issues) != 0 {
		return Result{}, invalid("invalid storage class")
	}
	quantity, err := resource.ParseQuantity(inputs["size"])
	if err != nil || quantity.Sign() <= 0 || quantity.Value() > 1<<40 {
		return Result{}, invalid("size must be a positive Kubernetes quantity up to 1Ti")
	}
	desired := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: map[string]string{"app.kubernetes.io/managed-by": managedByValue}},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			StorageClassName: &storageClass,
			Resources:        corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: quantity}},
		},
	}
	setPVCExecutionAnnotations(desired, execution, "pvc.deploy")
	claims := e.client.CoreV1().PersistentVolumeClaims(namespace)
	for attempt := 0; attempt < maxCASAttempts; attempt++ {
		existing, err := claims.Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			if created, err := claims.Create(ctx, desired.DeepCopy(), metav1.CreateOptions{}); err == nil {
				return pvcResult(created), nil
			} else if apierrors.IsAlreadyExists(err) || apierrors.IsConflict(err) {
				continue
			} else {
				return Result{}, targetError("create pvc", err)
			}
		}
		if err != nil {
			return Result{}, targetError("get pvc", err)
		}
		if err := checkManagedPVC(existing, execution); err != nil {
			return Result{}, err
		}
		storedGeneration, err := storedPVCFencingGeneration(existing)
		if err != nil {
			return Result{}, err
		}
		if storedGeneration > execution.FencingGeneration {
			return Result{}, fenced(storedGeneration, execution.FencingGeneration)
		}
		if storedGeneration == execution.FencingGeneration {
			if !exactPVCExecution(existing, execution) || !samePVCSpec(existing, desired) {
				return Result{}, conflict("equal-generation pvc deploy is not an exact replay")
			}
			return pvcResult(existing), nil
		}
		return Result{}, conflict("pvc already exists with an older generation")
	}
	return Result{}, fail(503, ErrorTargetUnavailable, true, "pvc changed during bounded CAS retries")
}

func setPVCExecutionAnnotations(claim *corev1.PersistentVolumeClaim, execution ExecutionContext, action string) {
	if claim.Annotations == nil {
		claim.Annotations = make(map[string]string)
	}
	claim.Annotations[managedByAnnotation] = managedByValue
	claim.Annotations[tenantAnnotation] = execution.TenantID
	claim.Annotations[operationAnnotation] = execution.OperationID
	claim.Annotations[stepAnnotation] = execution.StepID
	claim.Annotations[idempotencyAnnotation] = execution.IdempotencyKey
	claim.Annotations[fencingGenerationAnnotation] = strconv.FormatInt(execution.FencingGeneration, 10)
	claim.Annotations[executionAttemptIDAnnotation] = execution.ExecutionAttemptID
	claim.Annotations[lastActionAnnotation] = action
}

func checkManagedPVC(claim *corev1.PersistentVolumeClaim, execution ExecutionContext) error {
	a := claim.Annotations
	if a[managedByAnnotation] != managedByValue || a[tenantAnnotation] != execution.TenantID {
		return conflict("pvc is not owned by this tenant/provider")
	}
	return nil
}

func storedPVCFencingGeneration(claim *corev1.PersistentVolumeClaim) (int64, error) {
	value := claim.Annotations[fencingGenerationAnnotation]
	generation, err := strconv.ParseInt(value, 10, 64)
	if err != nil || generation <= 0 || strconv.FormatInt(generation, 10) != value {
		return 0, conflict("pvc has an invalid fencing generation")
	}
	return generation, nil
}

func exactPVCExecution(claim *corev1.PersistentVolumeClaim, execution ExecutionContext) bool {
	a := claim.Annotations
	return a[tenantAnnotation] == execution.TenantID && a[operationAnnotation] == execution.OperationID &&
		a[stepAnnotation] == execution.StepID && a[idempotencyAnnotation] == execution.IdempotencyKey &&
		a[executionAttemptIDAnnotation] == execution.ExecutionAttemptID && a[lastActionAnnotation] == "pvc.deploy"
}

func samePVCSpec(existing, desired *corev1.PersistentVolumeClaim) bool {
	return existing.Spec.StorageClassName != nil && desired.Spec.StorageClassName != nil &&
		*existing.Spec.StorageClassName == *desired.Spec.StorageClassName &&
		reflect.DeepEqual(existing.Spec.AccessModes, desired.Spec.AccessModes) &&
		existing.Spec.Resources.Requests.Storage().Cmp(*desired.Spec.Resources.Requests.Storage()) == 0
}

func pvcResult(claim *corev1.PersistentVolumeClaim) Result {
	return Result{Outputs: map[string]any{
		"namespace": claim.Namespace, "name": claim.Name, "uid": string(claim.UID),
		"resourceVersion": claim.ResourceVersion, "volumeName": claim.Spec.VolumeName,
		"phase": string(claim.Status.Phase), "action": "pvc.deploy",
	}, Checkpoint: fmt.Sprintf("pvc:%s/%s:%s", claim.Namespace, claim.Name, claim.UID)}
}

func (e *Executor) upgrade(ctx context.Context, execution ExecutionContext, namespace, name string) (Result, error) {
	inputs := stringInputs(execution.Inputs)
	source, target := inputs["source_image"], inputs["target_image"]
	if !pinnedImage(source) || !pinnedImage(target) {
		return Result{}, invalid("source_image and target_image must be digest-pinned")
	}
	if inputs["data_compatible"] != "true" {
		return Result{}, invalid("data_compatible must be true before upgrade")
	}
	if strings.TrimSpace(inputs["backup_ref"]) == "" || len(inputs["backup_ref"]) > 256 {
		return Result{}, invalid("backup_ref is required before upgrade")
	}
	if inputs["expected_uid"] == "" {
		return Result{}, invalid("expected_uid is required")
	}
	deployments := e.client.AppsV1().Deployments(namespace)
	for attempt := 0; attempt < maxCASAttempts; attempt++ {
		current, err := deployments.Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return Result{}, conflict("deployment does not exist for upgrade")
		}
		if err != nil {
			return Result{}, targetError("get deployment", err)
		}
		if err := checkManagedTenant(current, execution); err != nil {
			return Result{}, err
		}
		if string(current.UID) != inputs["expected_uid"] {
			return Result{}, conflict("expected_uid does not match deployment UID")
		}
		storedGeneration, err := storedFencingGeneration(current)
		if err != nil {
			return Result{}, err
		}
		if storedGeneration > execution.FencingGeneration {
			return Result{}, fenced(storedGeneration, execution.FencingGeneration)
		}
		if storedGeneration == execution.FencingGeneration {
			if !exactExecution(current, execution, "workload.upgrade") ||
				current.Annotations[sourceImageAnnotation] != source ||
				current.Annotations[targetImageAnnotation] != target ||
				len(current.Spec.Template.Spec.Containers) != 1 ||
				current.Spec.Template.Spec.Containers[0].Image != target {
				return Result{}, conflict("equal-generation upgrade is not an exact replay")
			}
			return e.waitRolledOut(ctx, execution, namespace, name, "workload.upgrade", "workload.upgraded")
		}
		if len(current.Spec.Template.Spec.Containers) != 1 || current.Spec.Template.Spec.Containers[0].Image != source {
			return Result{}, conflict("current workload image does not match source_image")
		}
		updated := current.DeepCopy()
		updated.Spec.Template.Spec.Containers[0].Image = target
		setExecutionAnnotations(updated, execution, "workload.upgrade")
		updated.Annotations[sourceImageAnnotation] = source
		updated.Annotations[targetImageAnnotation] = target
		updated.Annotations[backupRefAnnotation] = inputs["backup_ref"]
		if _, err := deployments.Update(ctx, updated, metav1.UpdateOptions{}); err == nil {
			return e.waitRolledOut(ctx, execution, namespace, name, "workload.upgrade", "workload.upgraded")
		} else if apierrors.IsConflict(err) || apierrors.IsNotFound(err) {
			continue
		} else {
			return Result{}, targetError("upgrade deployment", err)
		}
	}
	return Result{}, fail(503, ErrorTargetUnavailable, true, "deployment changed during bounded upgrade retries")
}

func pinnedImage(image string) bool {
	return strings.Contains(image, "@sha256:") && len(image) <= 512 && !strings.ContainsAny(image, " \t\r\n")
}

func (e *Executor) rollback(ctx context.Context, execution ExecutionContext, namespace, name string) (Result, error) {
	inputs := stringInputs(execution.Inputs)
	source, target := inputs["source_image"], inputs["target_image"]
	if !pinnedImage(source) || !pinnedImage(target) {
		return Result{}, invalid("source_image and target_image must be digest-pinned")
	}
	if inputs["data_irreversible"] != "false" {
		return Result{}, invalid("irreversible data migration blocks automatic rollback")
	}
	if strings.TrimSpace(inputs["backup_ref"]) == "" || len(inputs["backup_ref"]) > 256 {
		return Result{}, invalid("backup_ref is required for rollback")
	}
	if inputs["expected_uid"] == "" {
		return Result{}, invalid("expected_uid is required")
	}
	deployments := e.client.AppsV1().Deployments(namespace)
	for attempt := 0; attempt < maxCASAttempts; attempt++ {
		current, err := deployments.Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return Result{}, conflict("deployment does not exist for rollback")
		}
		if err != nil {
			return Result{}, targetError("get deployment", err)
		}
		if err := checkManagedTenant(current, execution); err != nil {
			return Result{}, err
		}
		if string(current.UID) != inputs["expected_uid"] {
			return Result{}, conflict("expected_uid does not match deployment UID")
		}
		storedGeneration, err := storedFencingGeneration(current)
		if err != nil {
			return Result{}, err
		}
		if storedGeneration > execution.FencingGeneration {
			return Result{}, fenced(storedGeneration, execution.FencingGeneration)
		}
		if storedGeneration == execution.FencingGeneration {
			if !exactExecution(current, execution, "workload.rollback") ||
				current.Annotations[sourceImageAnnotation] != source ||
				current.Annotations[targetImageAnnotation] != target ||
				len(current.Spec.Template.Spec.Containers) != 1 ||
				current.Spec.Template.Spec.Containers[0].Image != target {
				return Result{}, conflict("equal-generation rollback is not an exact replay")
			}
			return e.waitRolledOut(ctx, execution, namespace, name, "workload.rollback", "workload.rolledback")
		}
		if len(current.Spec.Template.Spec.Containers) != 1 || current.Spec.Template.Spec.Containers[0].Image != source {
			return Result{}, conflict("current workload image does not match source_image")
		}
		updated := current.DeepCopy()
		updated.Spec.Template.Spec.Containers[0].Image = target
		setExecutionAnnotations(updated, execution, "workload.rollback")
		updated.Annotations[sourceImageAnnotation] = source
		updated.Annotations[targetImageAnnotation] = target
		updated.Annotations[backupRefAnnotation] = inputs["backup_ref"]
		if _, err := deployments.Update(ctx, updated, metav1.UpdateOptions{}); err == nil {
			return e.waitRolledOut(ctx, execution, namespace, name, "workload.rollback", "workload.rolledback")
		} else if apierrors.IsConflict(err) || apierrors.IsNotFound(err) {
			continue
		} else {
			return Result{}, targetError("rollback deployment", err)
		}
	}
	return Result{}, fail(503, ErrorTargetUnavailable, true, "deployment changed during bounded rollback retries")
}

// waitRolledOut requires the new ReplicaSet to be fully rolled out. Old
// available pods alone must not make a failed upgrade look successful.
func (e *Executor) waitRolledOut(ctx context.Context, execution ExecutionContext, namespace, name, action, resultAction string) (Result, error) {
	var deployment *appsv1.Deployment
	err := wait.PollUntilContextTimeout(ctx, e.pollInterval, 90*time.Second, true, func(ctx context.Context) (bool, error) {
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
		if storedGeneration != execution.FencingGeneration || !exactExecution(current, execution, action) {
			return false, conflict("deployment changed while waiting for rollout")
		}
		deployment = current
		desired := int32(1)
		if current.Spec.Replicas != nil {
			desired = *current.Spec.Replicas
		}
		return current.Status.ObservedGeneration >= current.Generation &&
			current.Status.UpdatedReplicas >= desired &&
			current.Status.ReadyReplicas >= desired &&
			current.Status.AvailableReplicas >= desired &&
			current.Status.UnavailableReplicas == 0, nil
	})
	if err != nil {
		var statusErr *StatusError
		if errors.As(err, &statusErr) {
			return Result{}, statusErr
		}
		return Result{}, targetError("wait for deployment rollout", err)
	}
	return resourceResult(deployment, resultAction), nil
}

func (e *Executor) deletePVC(ctx context.Context, execution ExecutionContext, namespace, name string) (Result, error) {
	inputs := stringInputs(execution.Inputs)
	if inputs["data_cleanup_authorized"] != "true" {
		return Result{}, invalid("data cleanup requires explicit authorization")
	}
	if inputs["expected_uid"] == "" {
		return Result{}, invalid("expected_uid is required for data cleanup")
	}
	claims := e.client.CoreV1().PersistentVolumeClaims(namespace)
	claim, err := claims.Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return Result{}, conflict("pvc does not exist; expected_uid cannot be verified")
	}
	if err != nil {
		return Result{}, targetError("get pvc", err)
	}
	if err := checkManagedPVC(claim, execution); err != nil {
		return Result{}, err
	}
	if string(claim.UID) != inputs["expected_uid"] {
		return Result{}, conflict("expected_uid does not match pvc UID")
	}
	storedGeneration, err := storedPVCFencingGeneration(claim)
	if err != nil {
		return Result{}, err
	}
	if storedGeneration > execution.FencingGeneration {
		return Result{}, fenced(storedGeneration, execution.FencingGeneration)
	}
	if err := claims.Delete(ctx, name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &claim.UID}}); err != nil && !apierrors.IsNotFound(err) {
		return Result{}, targetError("delete pvc", err)
	}
	return Result{Outputs: map[string]any{
		"namespace": namespace, "name": name, "uid": string(claim.UID), "action": "pvc.deleted",
	}, Checkpoint: fmt.Sprintf("pvc-deleted:%s/%s:%s", namespace, name, claim.UID)}, nil
}
