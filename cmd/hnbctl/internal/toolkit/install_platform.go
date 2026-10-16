package toolkit

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// PlatformInstallResult records what the gated pipeline actually performed. It
// never asserts more than the recorded phases support.
type PlatformInstallResult struct {
	RunID     string `json:"runId"`
	Namespace string `json:"namespace"`
	Applied   bool   `json:"applied"`
	Verified  bool   `json:"verified"`
}

// RunPlatformInstall is the gated platform release pipeline. It refuses unless
// the supplied evidence matrix certifies the delivery, then applies the rendered
// manifests and verifies rollout, recording each phase in the run store. It does
// not run database migrations: doing so needs plane credentials, so a successful
// apply still leaves the platform layer incomplete rather than claiming success.
func RunPlatformInstall(ctx context.Context, store RunStore, writer string, epoch int64, cfg *DeploymentConfig,
	kubeconfig string, rendered []byte, workloads []WorkloadRef, runner ClusterRunner, evidence *EvidenceMatrix) (PlatformInstallResult, error) {
	result := PlatformInstallResult{}
	if cfg == nil {
		return result, fmt.Errorf("PLATFORM_NOT_CERTIFIED: no configuration")
	}
	result.Namespace = cfg.Spec.Platform.Namespace
	if reasons := EvidenceGate(cfg, evidence); len(reasons) > 0 {
		return result, fmt.Errorf("PLATFORM_NOT_CERTIFIED: %s", strings.Join(reasons, "; "))
	}
	if kubeconfig == "" || len(rendered) == 0 || runner == nil {
		return result, fmt.Errorf("PLATFORM_INPUT_INVALID: kubeconfig, rendered manifests and runner are required")
	}
	run, err := LoadRunState(store.Dir)
	if err != nil {
		return result, err
	}
	result.RunID = run.RunID
	applier := PlatformApplier{Kubeconfig: kubeconfig, Namespace: cfg.Spec.Platform.Namespace, Manifests: rendered,
		Workloads: workloads, Runner: runner, Timeout: cfg.Spec.Timeouts.Readiness}
	budget := cfg.Spec.Timeouts.Readiness
	if budget <= 0 {
		budget = 10 * time.Minute
	}
	if err := store.RunPhaseWithTimeout(ctx, writer, epoch, PhasePlatformRelease, budget, nil, nil, applier.Apply); err != nil {
		return result, err
	}
	result.Applied = true
	if err := store.RunPhaseWithTimeout(ctx, writer, epoch, PhasePlatformVerified, budget, nil, nil, applier.Verify); err != nil {
		return result, err
	}
	result.Verified = true
	return result, nil
}
