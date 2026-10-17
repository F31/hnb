package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"
)

// runAutoUninstall is the zero-config path: detect the HNB installation from
// the cluster and remove it without a recorded install run. -all also removes
// the bundled TopoLVM/StorageClass components; -data additionally removes
// PVCs, PVs, LogicalVolumes, Secrets and the namespace.
func runAutoUninstall(ctx context.Context, env *Env, namespace, kubeconfig string, all, deleteData, yes, planOnly bool, format string) Result {
	if namespace == "" {
		namespace = "hnb-system"
	}
	kubeconfig = resolveAutoKubeconfig(kubeconfig)
	if kubeconfig == "" {
		return fail(ReasonInvalidConfig, "no kubeconfig found; pass --kubeconfig (KUBECONFIG, ~/.kube/config, /etc/rancher/k3s/k3s.yaml are auto-detected)")
	}
	runner := toolkit.KubectlRunner{}
	detected, err := runner.Run(ctx, kubeconfig, nil, "get", "deployments", "--namespace", namespace,
		"--selector", "app.kubernetes.io/instance=hnb", "--output", "name")
	if err != nil || strings.TrimSpace(detected) == "" {
		detected, err = runner.Run(ctx, kubeconfig, nil, "get", "all", "--namespace", namespace, "--output", "name")
	}
	if err != nil || strings.TrimSpace(detected) == "" {
		// The namespace can already be gone while TopoLVM/StorageClass or data
		// volumes still remain. Treat those as HNB remnants for -all cleanup.
		for _, probe := range [][]string{
			{"get", "storageclass", "hnb-topolvm", "--output", "name"},
			{"get", "namespace", "topolvm-system", "--output", "name"},
			{"get", "crd", "logicalvolumes.topolvm.io", "--output", "name"},
		} {
			detected, err = runner.Run(ctx, kubeconfig, nil, probe...)
			if err == nil && strings.TrimSpace(detected) != "" {
				break
			}
		}
	}
	if err != nil || strings.TrimSpace(detected) == "" {
		return fail(ReasonInvalidConfig, "no HNB installation detected in namespace "+namespace)
	}

	plan := toolkit.RemovalPlan{
		SchemaVersion:        toolkit.RemovalPlanSchema,
		Mode:                 toolkit.RemovalUninstallHNB,
		Namespace:            namespace,
		Authorized:           true,
		DataLossAcknowledged: deleteData,
		GeneratedAt:          time.Now().UTC(),
		Delete: []string{
			"deployment,statefulset,daemonset,service,serviceaccount,role,rolebinding (namespace " + namespace + ")",
			"configmap (except kube-root-ca.crt) (namespace " + namespace + ")",
			"clusterrole/clusterrolebinding -l app.kubernetes.io/instance=hnb",
		},
		Retain: []string{
			"external CNI daemonsets/deployments (never modified or replaced)",
			"external kubernetes cluster lifecycle owner (kubeconfig is the entry point)",
			"external namespaces/pvs/business workloads outside " + namespace,
		},
	}
	if deleteData {
		plan.Delete = append(plan.Delete,
			"persistentvolumeclaim/* (namespace "+namespace+")",
			"persistentvolume/* (claims in namespace "+namespace+")",
			"logicalvolumes.topolvm.io/*",
			"secret/* (namespace "+namespace+")",
			"namespace/"+namespace)
	} else {
		plan.Retain = append(plan.Retain,
			"persistentvolumeclaim/* (namespace "+namespace+")",
			"persistentvolume/* (claims in namespace "+namespace+")",
			"secret/* (namespace "+namespace+")",
			"namespace/"+namespace)
	}
	if all {
		plan.Delete = append(plan.Delete,
			"namespace/topolvm-system",
			"storageclass/hnb-topolvm",
			"csidriver/topolvm.io",
			"clusterrole/clusterrolebinding -l app.kubernetes.io/name=topolvm")
		if deleteData {
			plan.Delete = append(plan.Delete, "crd/logicalvolumes.topolvm.io")
		} else {
			plan.Retain = append(plan.Retain, "logicalvolumes.topolvm.io (data retained)")
		}
	} else {
		plan.Retain = append(plan.Retain, "namespace/topolvm-system", "storageclass/hnb-topolvm", "csidriver/topolvm.io")
	}

	if planOnly {
		return writeAutoRemovalPlan(env, plan, format)
	}
	if !yes {
		toolkit.FormatRemovalHuman(env.Stderr, plan)
		message := "Delete HNB platform components in namespace " + namespace + "?"
		if deleteData {
			message = "DELETE ALL HNB DATA in namespace " + namespace + "? PVCs, PVs, LogicalVolumes, Secrets and the namespace will be permanently removed."
		}
		if !confirmOperation(env, message) {
			return fail(ReasonPreflightBlocked, "removal not confirmed; no changes made (use --yes for automation)")
		}
	}
	executor := toolkit.RemovalExecutor{
		Kubeconfig: kubeconfig, Namespace: namespace,
		Selector:   "app.kubernetes.io/instance=hnb",
		DeleteData: deleteData, DeleteAll: all,
	}
	result, err := executor.Execute(ctx, plan)
	if err != nil {
		return fail(ReasonPlatformNotVerified, err.Error())
	}
	if format == "json" {
		if err := json.NewEncoder(env.Stdout).Encode(result); err != nil {
			return fail(ReasonOutputFailed, "cannot write removal result")
		}
	} else {
		fmt.Fprintf(env.Stdout, "uninstalled HNB in %s (all=%v data=%v); verified=%v\n", namespace, all, deleteData, result.Verified)
	}
	return success("")
}

func writeAutoRemovalPlan(env *Env, plan toolkit.RemovalPlan, format string) Result {
	if format == "json" {
		if err := json.NewEncoder(env.Stdout).Encode(plan); err != nil {
			return fail(ReasonOutputFailed, "cannot write removal plan")
		}
		return success("")
	}
	toolkit.FormatRemovalHuman(env.Stdout, plan)
	return success("")
}

func resolveAutoKubeconfig(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if value := os.Getenv("KUBECONFIG"); value != "" {
		for _, candidate := range filepath.SplitList(value) {
			if candidate != "" {
				return candidate
			}
		}
	}
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, ".kube", "config"),
		"/etc/rancher/k3s/k3s.yaml",
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}
