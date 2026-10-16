package toolkit

import (
	"context"
	"fmt"
	"strings"
)

// RunManagementBootstrap is the gated Day-0 pipeline for a blank management node.
// It refuses unless the supplied evidence matrix certifies the delivery, then
// runs the management bootstrap adapter (node prerequisites through a verified
// cluster) and returns its result. It performs no platform release install: the
// rendered-release applier is a separate, existing-cluster step.
func RunManagementBootstrap(ctx context.Context, cfg *DeploymentConfig, lock *ReleaseLock, sources BootstrapSources,
	runID, runDir string, runner NodeProbeRunner, evidence *EvidenceMatrix) (ManagementBootstrapResult, error) {
	var result ManagementBootstrapResult
	if reasons := EvidenceGate(cfg, evidence); len(reasons) > 0 {
		return result, fmt.Errorf("PLATFORM_NOT_CERTIFIED: %s", strings.Join(reasons, "; "))
	}
	adapter, err := NewManagementBootstrap(cfg, lock, sources, runID, runDir, runner)
	if err != nil {
		return result, err
	}
	return adapter.Execute(ctx)
}
