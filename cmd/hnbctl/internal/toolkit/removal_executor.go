package toolkit

import (
	"context"
	"encoding/json"
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
	// DeleteData removes PVCs, their PVs, all Secrets and the namespace. It is
	// only set after explicit data-loss acknowledgement.
	DeleteData bool
	// DeleteAll also removes the bundled storage components (TopoLVM namespace,
	// StorageClass, CSIDriver and labeled cluster RBAC). When DeleteData is
	// false, PVCs/PVs/LVs are retained.
	DeleteAll        bool
	StorageClass     string
	TopolvmNamespace string
	TopolvmCRD       string
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

	// Capture PVs before PVC deletion; Retain policy leaves them Released.
	var pvNames []string
	if e.DeleteData {
		var err error
		pvNames, err = e.persistentVolumesForNamespace(ctx, runner)
		if err != nil {
			return result, err
		}
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

	if e.DeleteData {
		if _, err := runner.Run(ctx, e.Kubeconfig, nil,
			"delete", "persistentvolumeclaim", "--all", "--namespace", e.Namespace,
			"--ignore-not-found=true", "--wait=true", "--timeout="+timeout.String()); err != nil {
			return result, fmt.Errorf("REMOVAL_PVC_DELETE_FAILED")
		}
		result.Deleted = append(result.Deleted, "persistentvolumeclaim/* (namespace "+e.Namespace+")")
		if len(pvNames) > 0 {
			args := append([]string{"delete", "persistentvolume", "--ignore-not-found=true", "--wait=true", "--timeout=" + timeout.String()}, pvNames...)
			if _, err := runner.Run(ctx, e.Kubeconfig, nil, args...); err != nil {
				return result, fmt.Errorf("REMOVAL_PV_DELETE_FAILED")
			}
			result.Deleted = append(result.Deleted, "persistentvolume ("+strings.Join(pvNames, ",")+")")
		}
		// TopoLVM keeps LogicalVolume CRs after PV deletion; remove them so the
		// backing loop volumes are released.
		if _, err := runner.Run(ctx, e.Kubeconfig, nil,
			"delete", "logicalvolumes.topolvm.io", "--all", "--ignore-not-found=true", "--wait=true", "--timeout="+timeout.String()); err != nil {
			return result, fmt.Errorf("REMOVAL_LOGICALVOLUME_DELETE_FAILED")
		}
		result.Deleted = append(result.Deleted, "logicalvolumes.topolvm.io/*")
		if _, err := runner.Run(ctx, e.Kubeconfig, nil,
			"delete", "secret", "--all", "--namespace", e.Namespace,
			"--ignore-not-found=true", "--wait=true", "--timeout="+timeout.String()); err != nil {
			return result, fmt.Errorf("REMOVAL_SECRET_DELETE_FAILED")
		}
		result.Deleted = append(result.Deleted, "secret/* (namespace "+e.Namespace+")")
		if _, err := runner.Run(ctx, e.Kubeconfig, nil,
			"delete", "namespace", e.Namespace, "--ignore-not-found=true", "--wait=true", "--timeout="+timeout.String()); err != nil {
			return result, fmt.Errorf("REMOVAL_NAMESPACE_DELETE_FAILED")
		}
		result.Deleted = append(result.Deleted, "namespace/"+e.Namespace)
	}

	if e.DeleteAll {
		topolvmNamespace := e.TopolvmNamespace
		if topolvmNamespace == "" {
			topolvmNamespace = "topolvm-system"
		}
		storageClass := e.StorageClass
		if storageClass == "" {
			storageClass = "hnb-topolvm"
		}
		topolvmCRD := e.TopolvmCRD
		if topolvmCRD == "" {
			topolvmCRD = "logicalvolumes.topolvm.io"
		}
		if _, err := runner.Run(ctx, e.Kubeconfig, nil,
			"delete", "namespace", topolvmNamespace, "--ignore-not-found=true", "--wait=true", "--timeout="+timeout.String()); err != nil {
			return result, fmt.Errorf("REMOVAL_TOPOLVM_NAMESPACE_DELETE_FAILED")
		}
		result.Deleted = append(result.Deleted, "namespace/"+topolvmNamespace)
		for _, args := range [][]string{
			{"delete", "storageclass", storageClass, "--ignore-not-found=true"},
			{"delete", "csidriver", "topolvm.io", "--ignore-not-found=true"},
			{"delete", "clusterrole,clusterrolebinding", "-l", "app.kubernetes.io/name=topolvm", "--ignore-not-found=true"},
		} {
			if _, err := runner.Run(ctx, e.Kubeconfig, nil, args...); err != nil {
				return result, fmt.Errorf("REMOVAL_TOPOLVM_RESOURCE_DELETE_FAILED")
			}
		}
		result.Deleted = append(result.Deleted, "storageclass/"+storageClass, "csidriver/topolvm.io", "topolvm cluster RBAC")
		if e.DeleteData {
			if _, err := runner.Run(ctx, e.Kubeconfig, nil,
				"delete", "crd", topolvmCRD, "--ignore-not-found=true", "--wait=true", "--timeout="+timeout.String()); err != nil {
				return result, fmt.Errorf("REMOVAL_TOPOLVM_CRD_DELETE_FAILED")
			}
			result.Deleted = append(result.Deleted, "crd/"+topolvmCRD)
		}
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

	if e.DeleteData {
		out, err := runner.Run(ctx, e.Kubeconfig, nil, "get", "namespace", e.Namespace, "--ignore-not-found", "-o", "name")
		if err != nil {
			return result, fmt.Errorf("REMOVAL_VERIFY_FAILED")
		}
		if strings.TrimSpace(out) != "" {
			return result, fmt.Errorf("REMOVAL_INCOMPLETE: namespace %s still present", e.Namespace)
		}
		result.Verified = true
		result.Retained = append(result.Retained, plan.Retain...)
		return result, nil
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

func (e RemovalExecutor) persistentVolumesForNamespace(ctx context.Context, runner ClusterRunner) ([]string, error) {
	raw, err := runner.Run(ctx, e.Kubeconfig, nil, "get", "persistentvolume", "-o", "json")
	if err != nil {
		return nil, fmt.Errorf("REMOVAL_PV_LIST_FAILED")
	}
	var list struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Spec struct {
				ClaimRef *struct {
					Namespace string `json:"namespace"`
				} `json:"claimRef"`
			} `json:"spec"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, fmt.Errorf("REMOVAL_PV_LIST_INVALID")
	}
	var names []string
	for _, item := range list.Items {
		if item.Spec.ClaimRef != nil && item.Spec.ClaimRef.Namespace == e.Namespace {
			names = append(names, item.Metadata.Name)
		}
	}
	return names, nil
}
