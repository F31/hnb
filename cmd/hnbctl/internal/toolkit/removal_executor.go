package toolkit

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const RemovalExecutionSchema = "removal-execution/v1"

// RemovalExecution records what the local executor actually deleted. It is
// deliberately narrower than a removal plan: data-bearing resources are never
// deleted by the default uninstall path.
type RemovalExecution struct {
	SchemaVersion string   `json:"schemaVersion"`
	Namespace     string   `json:"namespace"`
	Deleted       []string `json:"deleted"`
	Retained      []string `json:"retained"`
	Verified      bool     `json:"verified"`
}

// RemovalExecutor executes a reviewed uninstall-hnb plan against an existing
// cluster. It deletes HNB workloads and RBAC, but retains the namespace, PVCs,
// Secrets, StorageClass and external/operator-owned resources. It never
// executes unmanage or delete-management-base locally.
type RemovalExecutor struct {
	Kubeconfig string
	Namespace  string
	Selector   string
	Runner     ClusterRunner
	Timeout    time.Duration
}

// Execute applies the plan. The caller must have acknowledged the operation.
func (e RemovalExecutor) Execute(ctx context.Context, plan RemovalPlan) (RemovalExecution, error) {
	result := RemovalExecution{SchemaVersion: RemovalExecutionSchema, Namespace: e.Namespace}
	if !plan.Authorized {
		return result, fmt.Errorf("REMOVAL_NOT_AUTHORIZED: %s", strings.Join(plan.Blocked, "; "))
	}
	if plan.Mode != RemovalUninstallHNB {
		return result, fmt.Errorf("REMOVAL_MODE_REQUIRES_PLATFORM: %s is server-authoritative and is not executed locally", plan.Mode)
	}
	if e.Kubeconfig == "" || e.Namespace == "" {
		return result, fmt.Errorf("REMOVAL_INPUT_INVALID: kubeconfig and namespace are required")
	}
	runner := e.Runner
	if runner == nil {
		runner = KubectlRunner{}
	}
	timeout := e.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	selector := e.Selector
	if selector == "" {
		selector = "app.kubernetes.io/instance=hnb"
	}

	// Workloads and namespaced RBAC. StatefulSet deletion does not delete PVCs.
	namespaced := "deployment,statefulset,daemonset,service,serviceaccount,role,rolebinding"
	if _, err := runner.Run(ctx, e.Kubeconfig, nil,
		"delete", namespaced, "--all", "--namespace", e.Namespace,
		"--ignore-not-found=true", "--wait=true", "--timeout="+timeout.String()); err != nil {
		return result, fmt.Errorf("REMOVAL_WORKLOAD_DELETE_FAILED")
	}
	result.Deleted = append(result.Deleted, namespaced+" (namespace "+e.Namespace+")")

	// ConfigMaps: keep the auto-managed kube-root-ca.crt, delete the rest.
	out, err := runner.Run(ctx, e.Kubeconfig, nil, "get", "configmap", "--namespace", e.Namespace, "-o", "name")
	if err != nil {
		return result, fmt.Errorf("REMOVAL_CONFIGMAP_LIST_FAILED")
	}
	var configMaps []string
	for _, line := range strings.Split(out, "\n") {
		name := strings.TrimSpace(line)
		if name == "" || strings.HasSuffix(name, "/kube-root-ca.crt") {
			continue
		}
		configMaps = append(configMaps, name)
	}
	if len(configMaps) > 0 {
		args := append([]string{"delete", "--namespace", e.Namespace, "--ignore-not-found=true", "--wait=true", "--timeout=" + timeout.String()}, configMaps...)
		if _, err := runner.Run(ctx, e.Kubeconfig, nil, args...); err != nil {
			return result, fmt.Errorf("REMOVAL_CONFIGMAP_DELETE_FAILED")
		}
		result.Deleted = append(result.Deleted, "configmap ("+strings.Join(configMaps, ",")+")")
	}

	// Cluster-scoped RBAC owned by this release. It is selected by the label,
	// never by deleting all cluster roles.
	for _, kind := range []string{"clusterrole", "clusterrolebinding"} {
		if _, err := runner.Run(ctx, e.Kubeconfig, nil,
			"delete", kind, "-l", selector, "--ignore-not-found=true", "--wait=true", "--timeout="+timeout.String()); err != nil {
			return result, fmt.Errorf("REMOVAL_CLUSTER_RBAC_DELETE_FAILED")
		}
		result.Deleted = append(result.Deleted, kind+" -l "+selector)
	}

	// Verify no HNB pods remain in the namespace. The namespace itself is kept.
	// Deletion can return before pods finish terminating, so poll until the
	// reviewed timeout rather than reporting a false incomplete result.
	deadline := time.Now().Add(timeout)
	for {
		pods, err := runner.Run(ctx, e.Kubeconfig, nil, "get", "pods", "--namespace", e.Namespace, "--no-headers")
		if err != nil {
			return result, fmt.Errorf("REMOVAL_VERIFY_FAILED")
		}
		trimmed := strings.TrimSpace(pods)
		if trimmed == "" || strings.Contains(trimmed, "No resources found") {
			break
		}
		if !time.Now().Before(deadline) {
			return result, fmt.Errorf("REMOVAL_INCOMPLETE: pods still present in namespace %s", e.Namespace)
		}
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	result.Verified = true
	result.Retained = append(result.Retained, plan.Retain...)
	return result, nil
}
