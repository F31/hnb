package network

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"k8s.io/klog/v2"

)

type KubeOVNProvider struct {
	helmPath string
}

func NewKubeOVNProvider(helmPath string) *KubeOVNProvider {
	return &KubeOVNProvider{helmPath: helmPath}
}

func (p *KubeOVNProvider) Name() string {
	return "kube-ovn"
}

func (p *KubeOVNProvider) Install(ctx context.Context, profile *NetworkProfile, target *RuntimeTarget) error {
	klog.Infof("[kube-ovn] installing Kube-OVN %s on target %s", profile.Version, target.ID)

	values, err := p.renderValues(profile)
	if err != nil {
		return fmt.Errorf("render values: %w", err)
	}

	args := []string{
		"upgrade", "--install", "kube-ovn", "kubeovn/kube-ovn",
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

	klog.Infof("[kube-ovn] installed successfully: %s", strings.TrimSpace(stdout.String()))
	return nil
}

func (p *KubeOVNProvider) Uninstall(ctx context.Context, profile *NetworkProfile, target *RuntimeTarget) error {
	klog.Infof("[kube-ovn] uninstalling from target %s", target.ID)

	args := []string{"uninstall", "kube-ovn", "--namespace", "kube-system"}
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

	klog.Infof("[kube-ovn] uninstalled: %s", strings.TrimSpace(stdout.String()))
	return nil
}

func (p *KubeOVNProvider) Upgrade(ctx context.Context, profile *NetworkProfile, target *RuntimeTarget, version string) error {
	if version == "" {
		version = profile.Version
	}
	klog.Infof("[kube-ovn] upgrading to %s on target %s", version, target.ID)

	values, err := p.renderValues(profile)
	if err != nil {
		return fmt.Errorf("render values: %w", err)
	}

	args := []string{
		"upgrade", "kube-ovn", "kubeovn/kube-ovn",
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

	klog.Infof("[kube-ovn] upgraded: %s", strings.TrimSpace(stdout.String()))
	return nil
}

func (p *KubeOVNProvider) Capability() NetworkCapability {
	return NetworkCapability{
		ProviderName:       "kube-ovn",
		SupportsPolicy:     true,
		SupportsEncryption: false,
		SupportsDualStack:  true,
		SupportsEgress:     true,
		SupportsIngress:    true,
		SupportsHubble:     false,
		SupportedModes:     []string{"geneve", "vxlan"},
		SupportedIPAMModes: []string{"centralized", "distributed"},
	}
}

func (p *KubeOVNProvider) Health(ctx context.Context, target *RuntimeTarget) error {
	klog.Infof("[kube-ovn] health check on target %s", target.ID)

	args := []string{"status", "kube-ovn", "--namespace", "kube-system"}
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

	klog.Infof("[kube-ovn] health check passed: %s", strings.TrimSpace(stdout.String()))
	return nil
}

func (p *KubeOVNProvider) renderValues(profile *NetworkProfile) (string, error) {
	vals := map[string]any{}

	vals["enableLB"] = true
	vals["enableNetworkPolicy"] = profile.EnablePolicy

	switch profile.EncapMode {
	case "vxlan":
		vals["networkType"] = "vxlan"
	default:
		vals["networkType"] = "geneve"
	}

	if profile.PodCIDR != "" {
		vals["cidr"] = profile.PodCIDR
	}
	if profile.ServiceCIDR != "" {
		vals["svcCidr"] = profile.ServiceCIDR
	}
	if profile.IPVersion == "dual-stack" || profile.IPv6PodCIDR != "" {
		vals["enableIPv6"] = true
		if profile.IPv6PodCIDR != "" {
			vals["cidr6"] = profile.IPv6PodCIDR
		}
	}
	if profile.MTU > 0 {
		vals["mtu"] = profile.MTU
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