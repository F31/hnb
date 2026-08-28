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

)

type CalicoProvider struct {
	helmPath      string
	calicoctlPath string
}

func NewCalicoProvider(helmPath string) *CalicoProvider {
	return &CalicoProvider{
		helmPath:      helmPath,
		calicoctlPath: "calicoctl",
	}
}

func (p *CalicoProvider) Name() string {
	return "calico"
}

func (p *CalicoProvider) Install(ctx context.Context, profile *NetworkProfile, target *RuntimeTarget) error {
	klog.Infof("[calico] installing Calico %s on target %s", profile.Version, target.ID)

	values, err := p.renderValues(profile)
	if err != nil {
		return fmt.Errorf("render values: %w", err)
	}

	args := []string{
		"upgrade", "--install", "calico", "projectcalico/tigera-operator",
		"--namespace", "calico-system",
		"--create-namespace",
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

	klog.Infof("[calico] installed successfully: %s", strings.TrimSpace(stdout.String()))
	return nil
}

func (p *CalicoProvider) Uninstall(ctx context.Context, profile *NetworkProfile, target *RuntimeTarget) error {
	klog.Infof("[calico] uninstalling from target %s", target.ID)

	args := []string{"uninstall", "calico", "--namespace", "calico-system"}
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

	klog.Infof("[calico] uninstalled: %s", strings.TrimSpace(stdout.String()))
	return nil
}

func (p *CalicoProvider) Upgrade(ctx context.Context, profile *NetworkProfile, target *RuntimeTarget, version string) error {
	if version == "" {
		version = profile.Version
	}
	klog.Infof("[calico] upgrading to %s on target %s", version, target.ID)

	values, err := p.renderValues(profile)
	if err != nil {
		return fmt.Errorf("render values: %w", err)
	}

	args := []string{
		"upgrade", "calico", "projectcalico/tigera-operator",
		"--namespace", "calico-system",
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

	klog.Infof("[calico] upgraded: %s", strings.TrimSpace(stdout.String()))
	return nil
}

func (p *CalicoProvider) Capability() NetworkCapability {
	return NetworkCapability{
		ProviderName:       "calico",
		SupportsPolicy:     true,
		SupportsEncryption: true,
		EncryptionType:     "wireguard",
		SupportsDualStack:  true,
		SupportsEgress:     true,
		SupportsIngress:    false,
		SupportsHubble:     false,
		SupportedModes:     []string{"vxlan", "ipip", "none"},
		SupportedIPAMModes: []string{"host-local", "kubernetes"},
	}
}

func (p *CalicoProvider) Health(ctx context.Context, target *RuntimeTarget) error {
	klog.Infof("[calico] health check on target %s", target.ID)

	args := []string{"status", "calico", "--namespace", "calico-system"}
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

	ds, err := clientset.AppsV1().DaemonSets("calico-system").Get(ctx, "calico-node", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get calico-node daemonset: %w", err)
	}

	if ds.Status.DesiredNumberScheduled != ds.Status.NumberReady {
		return fmt.Errorf("calico-node daemonset: %d desired, %d ready (not all nodes ready)",
			ds.Status.DesiredNumberScheduled, ds.Status.NumberReady)
	}

	klog.Infof("[calico] health check passed: %d/%d nodes ready",
		ds.Status.NumberReady, ds.Status.DesiredNumberScheduled)
	return nil
}

func (p *CalicoProvider) newK8sClient(target *RuntimeTarget) (*kubernetes.Clientset, error) {
	cfg, err := p.restConfig(target)
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(cfg)
}

func (p *CalicoProvider) newDynamicClient(target *RuntimeTarget) (dynamic.Interface, error) {
	cfg, err := p.restConfig(target)
	if err != nil {
		return nil, err
	}
	return dynamic.NewForConfig(cfg)
}

func (p *CalicoProvider) restConfig(target *RuntimeTarget) (*rest.Config, error) {
	if target.Kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", target.Kubeconfig)
	}
	return rest.InClusterConfig()
}

func (p *CalicoProvider) ApplyNetworkPolicy(ctx context.Context, policy *NetworkPolicy, target *RuntimeTarget) error {
	klog.Infof("[calico] applying network policy %s/%s on target %s", policy.Namespace, policy.Name, target.ID)

	client, err := p.newDynamicClient(target)
	if err != nil {
		return fmt.Errorf("k8s client: %w", err)
	}

	gvr := schema.GroupVersionResource{
		Group:    "projectcalico.org",
		Version:  "v3",
		Resource: "networkpolicies",
	}

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "projectcalico.org/v3",
			"kind":       "NetworkPolicy",
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
		anns := make(map[string]string)
		obj.SetAnnotations(anns)
		_, err = client.Resource(gvr).Namespace(ns).Create(ctx, obj, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create policy: %w", err)
		}
		klog.Infof("[calico] created policy %s/%s", ns, name)
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
	klog.Infof("[calico] updated policy %s/%s", ns, name)
	return nil
}

func (p *CalicoProvider) DeleteNetworkPolicy(ctx context.Context, name, namespace string, target *RuntimeTarget) error {
	klog.Infof("[calico] deleting network policy %s/%s on target %s", namespace, name, target.ID)

	client, err := p.newDynamicClient(target)
	if err != nil {
		return fmt.Errorf("k8s client: %w", err)
	}

	gvr := schema.GroupVersionResource{
		Group:    "projectcalico.org",
		Version:  "v3",
		Resource: "networkpolicies",
	}

	err = client.Resource(gvr).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("delete policy: %w", err)
	}

	klog.Infof("[calico] deleted policy %s/%s", namespace, name)
	return nil
}

func (p *CalicoProvider) ApplyClusterwidePolicy(ctx context.Context, policy *NetworkPolicy, target *RuntimeTarget) error {
	klog.Infof("[calico] applying global network policy %s on target %s", policy.Name, target.ID)

	client, err := p.newDynamicClient(target)
	if err != nil {
		return fmt.Errorf("k8s client: %w", err)
	}

	gvr := schema.GroupVersionResource{
		Group:    "projectcalico.org",
		Version:  "v3",
		Resource: "globalnetworkpolicies",
	}

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "projectcalico.org/v3",
			"kind":       "GlobalNetworkPolicy",
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
			return fmt.Errorf("get existing global policy: %w", err)
		}
		anns := make(map[string]string)
		obj.SetAnnotations(anns)
		_, err = client.Resource(gvr).Create(ctx, obj, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create global policy: %w", err)
		}
		klog.Infof("[calico] created global policy %s", name)
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
		return fmt.Errorf("update global policy: %w", err)
	}
	klog.Infof("[calico] updated global policy %s", name)
	return nil
}

func (p *CalicoProvider) DeleteClusterwidePolicy(ctx context.Context, name string, target *RuntimeTarget) error {
	klog.Infof("[calico] deleting global network policy %s on target %s", name, target.ID)

	client, err := p.newDynamicClient(target)
	if err != nil {
		return fmt.Errorf("k8s client: %w", err)
	}

	gvr := schema.GroupVersionResource{
		Group:    "projectcalico.org",
		Version:  "v3",
		Resource: "globalnetworkpolicies",
	}

	err = client.Resource(gvr).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("delete global policy: %w", err)
	}

	klog.Infof("[calico] deleted global policy %s", name)
	return nil
}

func (p *CalicoProvider) PolicyTrace(ctx context.Context, trace *PolicyTraceRequest, target *RuntimeTarget) (*PolicyTraceResult, error) {
	klog.Infof("[calico] policy trace on target %s", target.ID)

	args := []string{"policy", "trace", "--direction", trace.Direction}
	if trace.Verbose {
		args = append(args, "-v")
	}
	if trace.Source != nil {
		for k, v := range trace.Source {
			args = append(args, "--"+k, v)
		}
	}
	if trace.Destination != nil {
		for k, v := range trace.Destination {
			args = append(args, "--"+k, v)
		}
	}
	if target.Kubeconfig != "" {
		args = append(args, "--kubeconfig", target.Kubeconfig)
	}

	cmd := exec.CommandContext(ctx, p.calicoctlPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("calicoctl policy trace failed: %w\nstderr: %s", err, stderr.String())
	}

	output := strings.TrimSpace(stdout.String())
	verdict := "allowed"
	if strings.Contains(strings.ToLower(output), "denied") {
		verdict = "denied"
	}

	return &PolicyTraceResult{
		Verdict: verdict,
		Log:     output,
	}, nil
}

func (p *CalicoProvider) renderValues(profile *NetworkProfile) (string, error) {
	encap := "VXLAN"
	switch profile.EncapMode {
	case "ipip":
		encap = "IPIP"
	case "none":
		encap = "None"
	}

	ipPool := map[string]any{
		"cidr":          profile.PodCIDR,
		"encapsulation": encap,
	}
	if profile.IPVersion == "dual-stack" || profile.IPv6PodCIDR != "" {
		ipPool["natOutgoing"] = true
		ipPool["nodeSelector"] = "all()"
	}

	installation := map[string]any{
		"calicoNetwork": map[string]any{
			"ipPools": []any{ipPool},
			"mtu":     profile.MTU,
		},
	}

	if profile.IPVersion == "dual-stack" || profile.IPv6PodCIDR != "" {
		ipPool6 := map[string]any{
			"cidr":          profile.IPv6PodCIDR,
			"encapsulation": encap,
		}
		installation["calicoNetwork"].(map[string]any)["ipPools"] = []any{ipPool, ipPool6}
	}

	vals := map[string]any{
		"installation": installation,
	}

	if profile.EnablePolicy {
		vals["policy"] = map[string]any{
			"syncLabels": "Enabled",
		}
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