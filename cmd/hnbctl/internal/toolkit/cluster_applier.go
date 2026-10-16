package toolkit

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"
)

// ClusterRunner executes a cluster-facing command (kubectl) against a
// kubeconfig. It is the only seam through which the applier mutates a cluster,
// so tests can substitute a fixture without a real cluster.
type ClusterRunner interface {
	Run(ctx context.Context, kubeconfig string, stdin io.Reader, args ...string) (string, error)
}

// KubectlRunner shells out to kubectl. It never accepts credentials on argv: the
// kubeconfig path is passed as a flag and the token/exec plugin stays in the file.
type KubectlRunner struct {
	// Binary overrides the kubectl executable (default: "kubectl").
	Binary string
}

func (r KubectlRunner) Run(ctx context.Context, kubeconfig string, stdin io.Reader, args ...string) (string, error) {
	binary := r.Binary
	if binary == "" {
		binary = "kubectl"
	}
	full := append([]string{"--kubeconfig", kubeconfig}, args...)
	cmd := exec.CommandContext(ctx, binary, full...)
	cmd.Stdin = stdin
	output := &probeOutput{}
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Run(); err != nil {
		return output.String(), fmt.Errorf("cluster command failed")
	}
	return output.String(), nil
}

// WorkloadRef identifies a workload the applier waits to become ready.
type WorkloadRef struct {
	Kind string
	Name string
}

// PlatformApplier applies rendered platform manifests to a cluster and waits for
// rollout. It performs no rendering and no evidence decision of its own.
type PlatformApplier struct {
	Kubeconfig string
	Namespace  string
	Manifests  []byte
	Workloads  []WorkloadRef
	Runner     ClusterRunner
	Timeout    time.Duration
}

// Apply is idempotent: server-side apply reconciles the rendered resources.
func (a PlatformApplier) Apply(ctx context.Context) error {
	if a.Runner == nil || a.Kubeconfig == "" || a.Namespace == "" || len(a.Manifests) == 0 {
		return fmt.Errorf("APPLIER_INPUT_INVALID: kubeconfig, namespace, manifests and runner are required")
	}
	if _, err := a.Runner.Run(ctx, a.Kubeconfig, bytes.NewReader(a.Manifests),
		"apply", "--namespace", a.Namespace, "-f", "-"); err != nil {
		return fmt.Errorf("PLATFORM_APPLY_FAILED")
	}
	return nil
}

// Verify waits for each declared workload to reach rollout. Apply must run first.
func (a PlatformApplier) Verify(ctx context.Context) error {
	if a.Runner == nil || a.Kubeconfig == "" || a.Namespace == "" {
		return fmt.Errorf("APPLIER_INPUT_INVALID: kubeconfig, namespace and runner are required")
	}
	timeout := a.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	for _, w := range a.Workloads {
		if w.Kind == "" || w.Name == "" {
			return fmt.Errorf("APPLIER_INPUT_INVALID: workload kind and name are required")
		}
		if _, err := a.Runner.Run(ctx, a.Kubeconfig, nil,
			"--namespace", a.Namespace, "rollout", "status", w.Kind+"/"+w.Name, "--timeout", timeout.String()); err != nil {
			return fmt.Errorf("PLATFORM_ROLLOUT_FAILED: %s/%s", w.Kind, w.Name)
		}
	}
	return nil
}
