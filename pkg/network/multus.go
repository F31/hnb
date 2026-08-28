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

type MultusProvider struct {
	helmPath string
}

func NewMultusProvider(helmPath string) *MultusProvider {
	return &MultusProvider{helmPath: helmPath}
}

func (p *MultusProvider) Name() string {
	return "multus"
}

func (p *MultusProvider) Install(ctx context.Context, profile *NetworkProfile, target *RuntimeTarget) error {
	klog.Infof("[multus] installing Multus on target %s", target.ID)

	values, err := p.renderValues(profile)
	if err != nil {
		return fmt.Errorf("render values: %w", err)
	}

	args := []string{
		"upgrade", "--install", "multus", "k8snetworkplumbingwg/multus-cni",
		"--namespace", "kube-system",
		"--version", profile.Version,
		"--values", "-",
		"--wait",
		"--timeout", "10m",
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

	klog.Infof("[multus] installed successfully: %s", strings.TrimSpace(stdout.String()))
	return nil
}

func (p *MultusProvider) Uninstall(ctx context.Context, profile *NetworkProfile, target *RuntimeTarget) error {
	klog.Infof("[multus] uninstalling from target %s", target.ID)

	args := []string{"uninstall", "multus", "--namespace", "kube-system"}
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

	klog.Infof("[multus] uninstalled: %s", strings.TrimSpace(stdout.String()))
	return nil
}

func (p *MultusProvider) Upgrade(ctx context.Context, profile *NetworkProfile, target *RuntimeTarget, version string) error {
	if version == "" {
		version = profile.Version
	}
	klog.Infof("[multus] upgrading to %s on target %s", version, target.ID)

	values, err := p.renderValues(profile)
	if err != nil {
		return fmt.Errorf("render values: %w", err)
	}

	args := []string{
		"upgrade", "multus", "k8snetworkplumbingwg/multus-cni",
		"--namespace", "kube-system",
		"--version", version,
		"--values", "-",
		"--wait",
		"--timeout", "10m",
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

	klog.Infof("[multus] upgraded: %s", strings.TrimSpace(stdout.String()))
	return nil
}

func (p *MultusProvider) Capability() NetworkCapability {
	return NetworkCapability{
		ProviderName:       "multus",
		SupportsPolicy:     false,
		SupportsEncryption: false,
		SupportsDualStack:  true,
		SupportsEgress:     false,
		SupportsIngress:    false,
		SupportsHubble:     false,
		SupportedModes:     []string{"macvlan", "ipvlan", "sriov"},
		SupportedIPAMModes: []string{"host-local", "whereabouts"},
	}
}

func (p *MultusProvider) Health(ctx context.Context, target *RuntimeTarget) error {
	klog.Infof("[multus] health check on target %s", target.ID)

	args := []string{"status", "multus", "--namespace", "kube-system"}
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

	klog.Infof("[multus] health check passed: %s", strings.TrimSpace(stdout.String()))
	return nil
}

func (p *MultusProvider) renderValues(profile *NetworkProfile) (string, error) {
	vals := map[string]any{}

	vals["multus"] = map[string]any{
		"enabled": true,
	}

	if profile.IPVersion == "dual-stack" || profile.IPv6PodCIDR != "" {
		vals["multus"].(map[string]any)["ipv6"] = true
	}

	attach := make([]map[string]any, 0)

	if profile.ExtraConfig != nil {
		if networks, ok := profile.ExtraConfig["additional_networks"].([]any); ok {
			for _, n := range networks {
				if net, ok := n.(map[string]any); ok {
					attach = append(attach, net)
				}
			}
		}
	}

	if len(attach) > 0 {
		vals["additionalNetworks"] = attach
	}

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(vals); err != nil {
		return "", fmt.Errorf("encode values: %w", err)
	}

	return buf.String(), nil
}