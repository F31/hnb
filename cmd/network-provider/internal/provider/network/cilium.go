package network

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	"github.com/F31/hnb/cmd/network-provider/internal/provider"
)

type CiliumProvider struct {
	helmPath string
}

func NewCiliumProvider(helmPath string) *CiliumProvider {
	return &CiliumProvider{helmPath: helmPath}
}

func (p *CiliumProvider) Name() string {
	return "cilium"
}

func (p *CiliumProvider) Install(ctx context.Context, profile *provider.NetworkProfile, target *provider.RuntimeTarget) error {
	klog.Infof("[cilium] installing Cilium %s on target %s", profile.Version, target.ID)

	values, err := p.renderValues(profile)
	if err != nil {
		return fmt.Errorf("render values: %w", err)
	}

	args := []string{
		"upgrade", "--install", "cilium", "cilium/cilium",
		"--namespace", "kube-system",
		"--version", profile.Version,
		"--values", "-",
		"--wait",
		"--timeout", "15m",
	}

	if target.Kubeconfig != "" {
		args = append(args, "--kubeconfig", target.Kubeconfig)
	}

	cmd := exec.CommandContext(ctx, p.helmPath, args...)
	cmd.Stdin = strings.NewReader(values)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("helm install failed: %w\nstderr: %s", err, stderr.String())
	}

	klog.Infof("[cilium] installed successfully: %s", strings.TrimSpace(stdout.String()))

	if profile.IPVersion == "dual-stack" || profile.IPv6PodCIDR != "" {
		if err := p.postInstallDualStackVerify(ctx, target); err != nil {
			klog.Warningf("[cilium] dual-stack verification warning: %v", err)
		}
	}

	return nil
}

func (p *CiliumProvider) Uninstall(ctx context.Context, profile *provider.NetworkProfile, target *provider.RuntimeTarget) error {
	klog.Infof("[cilium] uninstalling from target %s", target.ID)

	args := []string{"uninstall", "cilium", "--namespace", "kube-system"}
	if target.Kubeconfig != "" {
		args = append(args, "--kubeconfig", target.Kubeconfig)
	}

	cmd := exec.CommandContext(ctx, p.helmPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("helm uninstall failed: %w\nstderr: %s", err, stderr.String())
	}

	klog.Infof("[cilium] uninstalled: %s", strings.TrimSpace(stdout.String()))
	return nil
}

func (p *CiliumProvider) Upgrade(ctx context.Context, profile *provider.NetworkProfile, target *provider.RuntimeTarget, version string) error {
	if version == "" {
		version = profile.Version
	}
	klog.Infof("[cilium] upgrading to %s on target %s", version, target.ID)

	if err := p.preUpgradeCheck(ctx, target); err != nil {
		return fmt.Errorf("pre-upgrade check failed: %w", err)
	}

	values, err := p.renderValues(profile)
	if err != nil {
		return fmt.Errorf("render values: %w", err)
	}

	args := []string{
		"upgrade", "cilium", "cilium/cilium",
		"--namespace", "kube-system",
		"--version", version,
		"--values", "-",
		"--wait",
		"--timeout", "15m",
		"--reset-values",
	}
	if target.Kubeconfig != "" {
		args = append(args, "--kubeconfig", target.Kubeconfig)
	}

	cmd := exec.CommandContext(ctx, p.helmPath, args...)
	cmd.Stdin = strings.NewReader(values)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("helm upgrade failed: %w\nstderr: %s", err, stderr.String())
	}

	if err := p.postUpgradeVerify(ctx, target); err != nil {
		return fmt.Errorf("post-upgrade verify failed: %w", err)
	}

	klog.Infof("[cilium] upgraded: %s", strings.TrimSpace(stdout.String()))
	return nil
}

func (p *CiliumProvider) Capability() provider.NetworkCapability {
	return provider.NetworkCapability{
		ProviderName:       "cilium",
		SupportsPolicy:     true,
		SupportsEncryption: true,
		EncryptionType:     "wireguard",
		SupportsDualStack:  true,
		SupportsEgress:     true,
		SupportsIngress:    true,
		SupportsHubble:     true,
		SupportedModes:     []string{"vxlan", "geneve", "direct-routing"},
		SupportedIPAMModes: []string{"cluster-pool", "kubernetes", "azure", "aws"},
	}
}

func (p *CiliumProvider) Health(ctx context.Context, target *provider.RuntimeTarget) error {
	klog.Infof("[cilium] health check on target %s", target.ID)

	args := []string{"status", "cilium", "--namespace", "kube-system"}
	if target.Kubeconfig != "" {
		args = append(args, "--kubeconfig", target.Kubeconfig)
	}

	cmd := exec.CommandContext(ctx, p.helmPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("helm release not found: %w\nstderr: %s", err, stderr.String())
	}

	clientset, err := p.newK8sClient(target)
	if err != nil {
		return fmt.Errorf("k8s client: %w", err)
	}

	ds, err := clientset.AppsV1().DaemonSets("kube-system").Get(ctx, "cilium", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get cilium daemonset: %w", err)
	}

	if ds.Status.DesiredNumberScheduled != ds.Status.NumberReady {
		return fmt.Errorf("cilium daemonset: %d desired, %d ready (not all nodes ready)",
			ds.Status.DesiredNumberScheduled, ds.Status.NumberReady)
	}

	klog.Infof("[cilium] health check passed: %d/%d nodes ready",
		ds.Status.NumberReady, ds.Status.DesiredNumberScheduled)
	return nil
}

func (p *CiliumProvider) preUpgradeCheck(ctx context.Context, target *provider.RuntimeTarget) error {
	klog.Infof("[cilium] pre-upgrade check on target %s", target.ID)

	args := []string{"status", "cilium", "--namespace", "kube-system"}
	if target.Kubeconfig != "" {
		args = append(args, "--kubeconfig", target.Kubeconfig)
	}
	cmd := exec.CommandContext(ctx, p.helmPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("helm release not found: %w\nstderr: %s", err, stderr.String())
	}

	clientset, err := p.newK8sClient(target)
	if err != nil {
		return fmt.Errorf("k8s client: %w", err)
	}

	ds, err := clientset.AppsV1().DaemonSets("kube-system").Get(ctx, "cilium", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get cilium daemonset: %w", err)
	}

	if ds.Status.DesiredNumberScheduled == 0 {
		return fmt.Errorf("cilium daemonset has 0 desired pods, nothing to upgrade")
	}

	if ds.Status.NumberReady == 0 {
		return fmt.Errorf("cilium daemonset: 0 ready pods, cannot upgrade")
	}

	if ds.Status.DesiredNumberScheduled != ds.Status.NumberReady {
		return fmt.Errorf("cilium daemonset not fully ready: %d/%d, upgrade blocked",
			ds.Status.NumberReady, ds.Status.DesiredNumberScheduled)
	}

	klog.Infof("[cilium] pre-upgrade check passed: %d/%d nodes ready",
		ds.Status.NumberReady, ds.Status.DesiredNumberScheduled)
	return nil
}

func (p *CiliumProvider) postUpgradeVerify(ctx context.Context, target *provider.RuntimeTarget) error {
	klog.Infof("[cilium] post-upgrade verify on target %s", target.ID)

	clientset, err := p.newK8sClient(target)
	if err != nil {
		return fmt.Errorf("k8s client: %w", err)
	}

	ds, err := clientset.AppsV1().DaemonSets("kube-system").Get(ctx, "cilium", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get cilium daemonset: %w", err)
	}

	if ds.Status.DesiredNumberScheduled != ds.Status.NumberReady {
		return fmt.Errorf("cilium daemonset not fully ready after upgrade: %d/%d",
			ds.Status.NumberReady, ds.Status.DesiredNumberScheduled)
	}

	if ds.Status.UpdatedNumberScheduled != ds.Status.DesiredNumberScheduled {
		return fmt.Errorf("cilium daemonset rollout not complete: %d updated, %d desired",
			ds.Status.UpdatedNumberScheduled, ds.Status.DesiredNumberScheduled)
	}

	klog.Infof("[cilium] post-upgrade verify passed: %d/%d nodes ready, rollout complete",
		ds.Status.NumberReady, ds.Status.DesiredNumberScheduled)
	return nil
}

func (p *CiliumProvider) ApplyNetworkPolicy(ctx context.Context, policy *provider.NetworkPolicy, target *provider.RuntimeTarget) error {
	klog.Infof("[cilium] applying network policy %s/%s on target %s", policy.Namespace, policy.Name, target.ID)

	client, err := p.newDynamicClient(target)
	if err != nil {
		return fmt.Errorf("k8s client: %w", err)
	}

	gvr := schema.GroupVersionResource{
		Group:    "cilium.io",
		Version:  "v2",
		Resource: "ciliumnetworkpolicies",
	}

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "cilium.io/v2",
			"kind":       "CiliumNetworkPolicy",
			"metadata": map[string]any{
				"name":      policy.Name,
				"namespace": policy.Namespace,
			},
			"spec": policy.Spec,
		},
	}

	if len(policy.Labels) > 0 {
		obj.Object["metadata"].(map[string]any)["labels"] = policy.Labels
	}

	ns := obj.GetNamespace()
	name := obj.GetName()

	existing, err := client.Resource(gvr).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("get existing policy: %w", err)
		}
		_, err = client.Resource(gvr).Namespace(ns).Create(ctx, obj, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create policy: %w", err)
		}
		klog.Infof("[cilium] created policy %s/%s", ns, name)
		return nil
	}

	anns := existing.GetAnnotations()
	if anns == nil {
		anns = make(map[string]string)
	}
	prevSpec, ok := existing.Object["spec"]
	if ok {
		specBytes, _ := json.Marshal(prevSpec)
		anns["hnb.cloud/previous-spec"] = string(specBytes)
	}
	obj.SetAnnotations(anns)

	obj.SetResourceVersion(existing.GetResourceVersion())
	_, err = client.Resource(gvr).Namespace(ns).Update(ctx, obj, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update policy: %w", err)
	}
	klog.Infof("[cilium] updated policy %s/%s (saved previous spec as annotation)", ns, name)
	return nil
}

func (p *CiliumProvider) RollbackNetworkPolicy(ctx context.Context, name, namespace string, target *provider.RuntimeTarget) error {
	klog.Infof("[cilium] rolling back network policy %s/%s on target %s", namespace, name, target.ID)

	client, err := p.newDynamicClient(target)
	if err != nil {
		return fmt.Errorf("k8s client: %w", err)
	}

	gvr := schema.GroupVersionResource{
		Group:    "cilium.io",
		Version:  "v2",
		Resource: "ciliumnetworkpolicies",
	}

	existing, err := client.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get policy: %w", err)
	}

	anns := existing.GetAnnotations()
	if anns == nil {
		return fmt.Errorf("no previous spec annotation found on %s/%s", namespace, name)
	}
	prevSpecStr, ok := anns["hnb.cloud/previous-spec"]
	if !ok || prevSpecStr == "" {
		return fmt.Errorf("no previous spec annotation found on %s/%s", namespace, name)
	}

	var prevSpec map[string]any
	if err := json.Unmarshal([]byte(prevSpecStr), &prevSpec); err != nil {
		return fmt.Errorf("unmarshal previous spec: %w", err)
	}

	existing.Object["spec"] = prevSpec
	_, err = client.Resource(gvr).Namespace(namespace).Update(ctx, existing, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("rollback policy: %w", err)
	}

	klog.Infof("[cilium] rolled back policy %s/%s to previous spec", namespace, name)
	return nil
}

func (p *CiliumProvider) PreviewNetworkPolicy(ctx context.Context, policy *provider.NetworkPolicy, target *provider.RuntimeTarget) (*provider.PolicyTraceResult, error) {
	klog.Infof("[cilium] preview network policy %s/%s on target %s", policy.Namespace, policy.Name, target.ID)

	return p.PolicyTrace(ctx, &provider.PolicyTraceRequest{
		Direction: "ingress",
		Verbose:   true,
	}, target)
}

func (p *CiliumProvider) postInstallDualStackVerify(ctx context.Context, target *provider.RuntimeTarget) error {
	klog.Infof("[cilium] dual-stack verification on target %s", target.ID)

	clientset, err := p.newK8sClient(target)
	if err != nil {
		return fmt.Errorf("k8s client: %w", err)
	}

	ds, err := clientset.AppsV1().DaemonSets("kube-system").Get(ctx, "cilium", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get cilium daemonset: %w", err)
	}

	env := ds.Spec.Template.Spec.Containers[0].Env
	ipv6Enabled := false
	for _, e := range env {
		if e.Name == "IPV6_ENABLE" && e.Value == "true" {
			ipv6Enabled = true
			break
		}
	}
	if ipv6Enabled {
		ips, err := clientset.CoreV1().Pods("kube-system").List(ctx, metav1.ListOptions{
			LabelSelector: "k8s-app=cilium",
		})
		if err != nil {
			return fmt.Errorf("list cilium pods: %w", err)
		}
		for _, pod := range ips.Items {
			for _, podIP := range pod.Status.PodIPs {
				if containsColon(podIP.IP) {
					klog.Infof("[cilium] dual-stack verified: pod %s has IPv6 address %s", pod.Name, podIP.IP)
					return nil
				}
			}
		}
		return fmt.Errorf("dual-stack enabled but no cilium pod has IPv6 address")
	}

	klog.Infof("[cilium] dual-stack not configured, skipping verification")
	return nil
}

func containsColon(s string) bool {
	for _, c := range s {
		if c == ':' {
			return true
		}
	}
	return false
}

func (p *CiliumProvider) ApplyClusterwidePolicy(ctx context.Context, policy *provider.NetworkPolicy, target *provider.RuntimeTarget) error {
	klog.Infof("[cilium] applying clusterwide policy %s on target %s", policy.Name, target.ID)

	client, err := p.newDynamicClient(target)
	if err != nil {
		return fmt.Errorf("k8s client: %w", err)
	}

	gvr := schema.GroupVersionResource{
		Group:    "cilium.io",
		Version:  "v2",
		Resource: "ciliumclusterwidepolicies",
	}

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "cilium.io/v2",
			"kind":       "CiliumClusterwideNetworkPolicy",
			"metadata": map[string]any{
				"name": policy.Name,
			},
			"spec": policy.Spec,
		},
	}

	if len(policy.Labels) > 0 {
		obj.Object["metadata"].(map[string]any)["labels"] = policy.Labels
	}

	name := obj.GetName()

	existing, err := client.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("get existing clusterwide policy: %w", err)
		}
		anns := make(map[string]string)
		obj.SetAnnotations(anns)
		_, err = client.Resource(gvr).Create(ctx, obj, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create clusterwide policy: %w", err)
		}
		klog.Infof("[cilium] created clusterwide policy %s", name)
		return nil
	}

	anns := existing.GetAnnotations()
	if anns == nil {
		anns = make(map[string]string)
	}
	prevSpec, ok := existing.Object["spec"]
	if ok {
		specBytes, _ := json.Marshal(prevSpec)
		anns["hnb.cloud/previous-spec"] = string(specBytes)
	}
	obj.SetAnnotations(anns)
	obj.SetResourceVersion(existing.GetResourceVersion())

	_, err = client.Resource(gvr).Update(ctx, obj, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update clusterwide policy: %w", err)
	}
	klog.Infof("[cilium] updated clusterwide policy %s", name)
	return nil
}

func (p *CiliumProvider) DeleteClusterwidePolicy(ctx context.Context, name string, target *provider.RuntimeTarget) error {
	klog.Infof("[cilium] deleting clusterwide policy %s on target %s", name, target.ID)

	client, err := p.newDynamicClient(target)
	if err != nil {
		return fmt.Errorf("k8s client: %w", err)
	}

	gvr := schema.GroupVersionResource{
		Group:    "cilium.io",
		Version:  "v2",
		Resource: "ciliumclusterwidepolicies",
	}

	err = client.Resource(gvr).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("delete clusterwide policy: %w", err)
	}

	klog.Infof("[cilium] deleted clusterwide policy %s", name)
	return nil
}

func (p *CiliumProvider) ApplyEnvoyConfig(ctx context.Context, config *provider.EnvoyConfig, target *provider.RuntimeTarget) error {
	klog.Infof("[cilium] applying envoy config %s/%s on target %s", config.Namespace, config.Name, target.ID)

	client, err := p.newDynamicClient(target)
	if err != nil {
		return fmt.Errorf("k8s client: %w", err)
	}

	gvr := schema.GroupVersionResource{
		Group:    "cilium.io",
		Version:  "v2",
		Resource: "ciliumenvoyconfigs",
	}

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "cilium.io/v2",
			"kind":       "CiliumEnvoyConfig",
			"metadata": map[string]any{
				"name":      config.Name,
				"namespace": config.Namespace,
			},
			"spec": config.Spec,
		},
	}

	if len(config.Labels) > 0 {
		obj.Object["metadata"].(map[string]any)["labels"] = config.Labels
	}

	ns := obj.GetNamespace()
	name := obj.GetName()

	existing, err := client.Resource(gvr).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("get existing envoy config: %w", err)
		}
		_, err = client.Resource(gvr).Namespace(ns).Create(ctx, obj, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create envoy config: %w", err)
		}
		klog.Infof("[cilium] created envoy config %s/%s", ns, name)
		return nil
	}

	obj.SetResourceVersion(existing.GetResourceVersion())
	_, err = client.Resource(gvr).Namespace(ns).Update(ctx, obj, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update envoy config: %w", err)
	}
	klog.Infof("[cilium] updated envoy config %s/%s", ns, name)
	return nil
}

func (p *CiliumProvider) DeleteEnvoyConfig(ctx context.Context, name, namespace string, target *provider.RuntimeTarget) error {
	klog.Infof("[cilium] deleting envoy config %s/%s on target %s", namespace, name, target.ID)

	client, err := p.newDynamicClient(target)
	if err != nil {
		return fmt.Errorf("k8s client: %w", err)
	}

	gvr := schema.GroupVersionResource{
		Group:    "cilium.io",
		Version:  "v2",
		Resource: "ciliumenvoyconfigs",
	}

	err = client.Resource(gvr).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("delete envoy config: %w", err)
	}

	klog.Infof("[cilium] deleted envoy config %s/%s", namespace, name)
	return nil
}

func (p *CiliumProvider) PolicyTrace(ctx context.Context, trace *provider.PolicyTraceRequest, target *provider.RuntimeTarget) (*provider.PolicyTraceResult, error) {
	klog.Infof("[cilium] policy trace on target %s", target.ID)

	args := []string{"policy", "trace", "--namespace", "kube-system", "--direction", trace.Direction}
	if trace.Verbose {
		args = append(args, "-v")
	}
	if trace.Source != nil {
		for k, v := range trace.Source {
			args = append(args, "--src-"+k, v)
		}
	}
	if trace.Destination != nil {
		for k, v := range trace.Destination {
			args = append(args, "--dst-"+k, v)
		}
	}
	if target.Kubeconfig != "" {
		args = append(args, "--kubeconfig", target.Kubeconfig)
	}

	cmd := exec.CommandContext(ctx, p.helmPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("cilium policy trace failed: %w\nstderr: %s", err, stderr.String())
	}

	output := strings.TrimSpace(stdout.String())
	verdict := "allowed"
	if strings.Contains(strings.ToLower(output), "denied") {
		verdict = "denied"
	}

	return &provider.PolicyTraceResult{
		Verdict: verdict,
		Log:     output,
	}, nil
}

func (p *CiliumProvider) newDynamicClient(target *provider.RuntimeTarget) (dynamic.Interface, error) {
	cfg, err := p.restConfig(target)
	if err != nil {
		return nil, err
	}
	return dynamic.NewForConfig(cfg)
}

func (p *CiliumProvider) newK8sClient(target *provider.RuntimeTarget) (*kubernetes.Clientset, error) {
	cfg, err := p.restConfig(target)
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(cfg)
}

func (p *CiliumProvider) restConfig(target *provider.RuntimeTarget) (*rest.Config, error) {
	if target.Kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", target.Kubeconfig)
	}
	return rest.InClusterConfig()
}

func (p *CiliumProvider) renderValues(profile *provider.NetworkProfile) (string, error) {
	vals := map[string]any{}

	if profile.IPVersion == "dual-stack" || profile.IPv6PodCIDR != "" {
		vals["ipv4"] = map[string]bool{"enabled": true}
		vals["ipv6"] = map[string]bool{"enabled": true}
	} else {
		vals["ipv4"] = map[string]bool{"enabled": true}
		vals["ipv6"] = map[string]bool{"enabled": false}
	}

	switch profile.EncapMode {
	case "vxlan":
		vals["tunnelProtocol"] = "vxlan"
	case "geneve":
		vals["tunnelProtocol"] = "geneve"
	case "direct-routing":
		vals["routingMode"] = "native"
	}

	switch profile.RoutingMode {
	case "native":
		vals["routingMode"] = "native"
	default:
		vals["routingMode"] = "tunnel"
	}

	if profile.MTU > 0 {
		vals["mtu"] = profile.MTU
	}

	vals["l7Proxy"] = profile.EnablePolicy
	vals["gatewayAPI"] = map[string]bool{"enabled": profile.EnablePolicy}

	hubbleRelay := profile.HubbleRelay || profile.EnableHubble
	hubbleUI := profile.HubbleUI || profile.EnableHubble
	hubbleVals := map[string]any{
		"enabled": profile.EnableHubble || profile.HubbleRelay || profile.HubbleUI,
		"relay":   map[string]bool{"enabled": hubbleRelay},
		"ui":      map[string]bool{"enabled": hubbleUI},
	}
	if len(profile.HubbleMetrics) > 0 {
		hubbleVals["metrics"] = map[string]any{
			"enabled": true,
			"port":    9965,
		}
		hubbleVals["listenAddress"] = ":9965"
	}
	vals["hubble"] = hubbleVals

	if profile.EnableOTel && profile.OTelTarget != "" {
		vals["opentelemetry"] = map[string]any{
			"enabled": true,
			"export": map[string]any{
				"target": profile.OTelTarget,
			},
		}
		vals["hubble"].(map[string]any)["export"] = map[string]any{
			"dynamic": true,
		}
	}

	if profile.EnableClusterMesh {
		cm := map[string]any{
			"enabled":       true,
			"clusterID":     profile.ClusterMeshID,
			"clusterName":   profile.ClusterMeshName,
		}
		if len(profile.ClusterMeshPeers) > 0 {
			peers := make([]map[string]any, 0, len(profile.ClusterMeshPeers))
			for _, peer := range profile.ClusterMeshPeers {
				peers = append(peers, map[string]any{
					"clusterID":   peer.ClusterID,
					"clusterName": peer.ClusterName,
					"endpoint":    peer.Endpoint,
				})
			}
			cm["remoteCluster"] = peers
		}
		vals["clustermesh"] = cm
		vals["tunnelProtocol"] = "vxlan"
	}

	switch profile.KubeProxyReplacement {
	case "strict":
		vals["kubeProxyReplacement"] = "strict"
	case "partial":
		vals["kubeProxyReplacement"] = "partial"
	default:
		vals["kubeProxyReplacement"] = "disabled"
	}

	switch profile.IPAMMode {
	case "cluster-pool":
		ipam := map[string]any{"mode": "cluster-pool"}
		if profile.PodCIDR != "" {
			ipam["operator"] = map[string]any{
				"clusterPoolIPv4PodCIDRList": []string{profile.PodCIDR},
			}
		}
		if profile.IPv6PodCIDR != "" {
			if ipam["operator"] == nil {
				ipam["operator"] = map[string]any{}
			}
			ipam["operator"].(map[string]any)["clusterPoolIPv6PodCIDRList"] = []string{profile.IPv6PodCIDR}
		}
		vals["ipam"] = ipam
	case "kubernetes":
		vals["ipam"] = map[string]string{"mode": "kubernetes"}
	default:
		vals["ipam"] = map[string]string{"mode": "cluster-pool"}
	}

	if profile.ServiceCIDR != "" {
		vals["k8sServiceHost"] = profile.ServiceCIDR
	}

	for k, v := range profile.ExtraConfig {
		vals[k] = v
	}

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(vals); err != nil {
		return "", fmt.Errorf("encode values: %w", err)
	}

	return buf.String(), nil
}