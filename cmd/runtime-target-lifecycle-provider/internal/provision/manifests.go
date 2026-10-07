package provision

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// CAPI API group/versions as rendered by this provider.
const (
	clusterAPIGroup       = "cluster.x-k8s.io"
	clusterAPIVersion     = "v1beta1"
	kubeadmBootstrapGroup = "bootstrap.cluster.x-k8s.io"
	kubeadmBootstrapVer   = "v1beta1"
	infraAPIGroup         = "infrastructure.cluster.x-k8s.io"
	infraAPIVersion       = "v1beta1"
)

var (
	clusterGVK              = schema.GroupVersionKind{Group: clusterAPIGroup, Version: clusterAPIVersion, Kind: "Cluster"}
	kubeadmControlPlaneGVK  = schema.GroupVersionKind{Group: clusterAPIGroup, Version: clusterAPIVersion, Kind: "KubeadmControlPlane"}
	kubeadmConfigTmplGVK    = schema.GroupVersionKind{Group: kubeadmBootstrapGroup, Version: kubeadmBootstrapVer, Kind: "KubeadmConfigTemplate"}
	machineDeploymentGVK    = schema.GroupVersionKind{Group: clusterAPIGroup, Version: clusterAPIVersion, Kind: "MachineDeployment"}
	sshClusterGVK           = schema.GroupVersionKind{Group: infraAPIGroup, Version: infraAPIVersion, Kind: "SSHCluster"}
	sshMachineTemplateGVK   = schema.GroupVersionKind{Group: infraAPIGroup, Version: infraAPIVersion, Kind: "SSHMachineTemplate"}
	sshMachineDeploymentGVK = schema.GroupVersionKind{Group: infraAPIGroup, Version: infraAPIVersion, Kind: "SSHMachineDeployment"}
)

func object(kind schema.GroupVersionKind, ns, name string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(kind)
	u.SetNamespace(ns)
	u.SetName(name)
	return u
}

func objectRef(kind schema.GroupVersionKind, ns, name string) map[string]any {
	return map[string]any{
		"apiVersion": kind.Group + "/" + kind.Version,
		"kind":       kind.Kind,
		"namespace":  ns,
		"name":       name,
	}
}

// CAPIManifests renders a CAPI work-load cluster (Cluster, KubeadmControlPlane,
// KubeadmConfigTemplate, MachineDeployment) plus the SSH infrastructure objects
// (SSHCluster + SSHMachineTemplate/Deployment) that this platform's provisioner
// reconciles. The manifests are applied to a management cluster when
// MANAGEMENT_KUBECONFIG is configured; without one they document the intended
// state so audit/DR tooling can replay them later.
//
// `clusterName` is the CAPI object name; it must be DNS-1123 low-case as seen by
// the management cluster. `bootstrapCloudInit` is the rendered KubeadmConfig
// cloud-init content (the kubeadm bootstrap provider contract); it is optional
// here — when empty the SSHMachine carries the join bootstrap directly.
func CAPIManifests(spec ProvisionSpec, clusterName string, bootstrapCloudInit string) []unstructured.Unstructured {
	ns := "default" // management cluster namespace (caller may override)
	objects := []unstructured.Unstructured{}

	// --- infra SSHCluster ---
	infraCluster := object(sshClusterGVK, ns, clusterName+"-ssh")
	infraCluster.Object["spec"] = map[string]any{
		"endpoint": spec.Endpoint(),
		"version":  spec.DesiredVersion,
		"nodes":    nodeList(spec.Nodes),
	}
	objects = append(objects, *infraCluster)

	// --- Cluster ---
	cluster := object(clusterGVK, ns, clusterName)
	cluster.Object["spec"] = map[string]any{
		"clusterNetwork": map[string]any{
			"pods":     map[string]any{"cidrBlocks": []string{defaultString(spec.PodCIDR, "10.244.0.0/16")}},
			"services": map[string]any{"cidrBlocks": []string{defaultString(spec.ServiceCIDR, "10.96.0.0/12")}},
		},
		"controlPlaneRef":   objectRef(kubeadmControlPlaneGVK, ns, clusterName+"-control-plane"),
		"infrastructureRef": objectRef(sshClusterGVK, ns, clusterName+"-ssh"),
	}
	objects = append(objects, *cluster)

	// --- KubeadmControlPlane + SSHMachineTemplate ; machine count = control-plane nodes ---
	kcp := object(kubeadmControlPlaneGVK, ns, clusterName+"-control-plane")
	kcp.Object["spec"] = map[string]any{
		"version":         spec.DesiredVersion,
		"replicas":        int64(len(spec.ControlPlaneNodes())),
		"machineTemplate": map[string]any{"infrastructureRef": objectRef(sshMachineTemplateGVK, ns, clusterName+"-cp-template")},
		"kubeadmConfigSpec": map[string]any{
			"clusterConfiguration": map[string]any{
				"controlPlaneEndpoint": spec.Endpoint(),
				"networking": map[string]any{
					"podSubnet":     defaultString(spec.PodCIDR, "10.244.0.0/16"),
					"serviceSubnet": defaultString(spec.ServiceCIDR, "10.96.0.0/12"),
					"dnsDomain":     defaultString(spec.ClusterDomain, "cluster.local"),
				},
			},
			"initConfiguration": map[string]any{
				"nodeRegistration": map[string]any{
					"kubeletExtraArgs": map[string]any{"node-ip": cpIP(spec)},
				},
			},
		},
	}
	if bootstrapCloudInit != "" {
		kcp.Object["spec"].(map[string]any)["kubeadmConfigSpec"].(map[string]any)["preKubeadmCommands"] = []string{bootstrapCloudInit}
	}
	objects = append(objects, *kcp)

	sshCPTemplate := object(sshMachineTemplateGVK, ns, clusterName+"-cp-template")
	sshCPTemplate.Object["spec"] = map[string]any{
		"role":  "control-plane",
		"nodes": nodeList(spec.ControlPlaneNodes()),
	}
	objects = append(objects, *sshCPTemplate)

	// --- MachineDeployment + KubeadmConfigTemplate + SSHMachineDeployment ---
	if len(spec.WorkerNodes()) > 0 {
		md := object(machineDeploymentGVK, ns, clusterName+"-workers")
		md.Object["spec"] = map[string]any{
			"replicas": int64(len(spec.WorkerNodes())),
			"selector": map[string]any{"matchLabels": map[string]any{"cluster.x-k8s.io/cluster-name": clusterName}},
			"template": map[string]any{
				"metadata": map[string]any{"labels": map[string]any{"cluster.x-k8s.io/cluster-name": clusterName}},
				"spec": map[string]any{
					"clusterName":       clusterName,
					"bootstrap":         map[string]any{"configRef": objectRef(kubeadmConfigTmplGVK, ns, clusterName+"-workers-template")},
					"infrastructureRef": objectRef(sshMachineDeploymentGVK, ns, clusterName+"-workers-infra"),
				},
			},
		}
		objects = append(objects, *md)

		kct := object(kubeadmConfigTmplGVK, ns, clusterName+"-workers-template")
		kct.Object["spec"] = map[string]any{"template": map[string]any{
			"spec": map[string]any{
				"joinConfiguration": map[string]any{
					"nodeRegistration": map[string]any{"kubeletExtraArgs": map[string]any{}},
				},
			},
		}}
		objects = append(objects, *kct)

		sshWorkersInfra := object(sshMachineDeploymentGVK, ns, clusterName+"-workers-infra")
		sshWorkersInfra.Object["spec"] = map[string]any{
			"role":  "worker",
			"nodes": nodeList(spec.WorkerNodes()),
		}
		objects = append(objects, *sshWorkersInfra)
	}

	return objects
}

func cpIP(spec ProvisionSpec) string {
	if cps := spec.ControlPlaneNodes(); len(cps) > 0 {
		return cps[0].Host
	}
	return ""
}

func defaultString(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func nodeList(spec []NodeSpec) []map[string]any {
	out := make([]map[string]any, 0, len(spec))
	for _, n := range spec {
		entry := map[string]any{
			"id":   n.ID,
			"host": n.Host,
			"port": n.EffectivePort(),
			"role": string(n.Role),
		}
		if n.User != "" {
			entry["user"] = n.User
		}
		if len(n.Labels) > 0 {
			entry["labels"] = n.Labels
		}
		out = append(out, entry)
	}
	return out
}
