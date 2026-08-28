package market

import (
	"crypto/sha256"
	"fmt"
)

type ManifestBridge struct{}

func NewManifestBridge() *ManifestBridge {
	return &ManifestBridge{}
}

func (mb *ManifestBridge) ToExecutionPlan(manifest *ReleaseManifest, policyResult *PolicyResult) (*ExecutionPlan, error) {
	if err := mb.validate(manifest); err != nil {
		return nil, err
	}

	steps := mb.buildSteps(manifest)
	pg := newPlanGenerator()

	plan, err := pg.generatePlan(manifest.ReleaseID, steps, mb.artifactDigests(manifest), policyResult)
	if err != nil {
		return nil, fmt.Errorf("generate plan: %w", err)
	}

	if err := pg.validatePlan(plan); err != nil {
		return nil, fmt.Errorf("validate plan: %w", err)
	}

	return plan, nil
}

func (mb *ManifestBridge) validate(manifest *ReleaseManifest) error {
	if manifest.ReleaseID == "" {
		return fmt.Errorf("release_id is required")
	}
	if manifest.Version == "" {
		return fmt.Errorf("version is required")
	}
	return nil
}

func (mb *ManifestBridge) buildSteps(manifest *ReleaseManifest) []StepSpec {
	pkgStepIDs := make(map[string]string, len(manifest.Packages))

	for _, pkg := range manifest.Packages {
		stepID := fmt.Sprintf("deploy-%s", pkg.Name)
		stepHash := sha256.Sum256([]byte(stepID))
		pkgStepIDs[pkg.Name] = fmt.Sprintf("%x", stepHash[:8])
	}

	var steps []StepSpec

	for _, pkg := range manifest.Packages {
		step := StepSpec{
			ID:       pkgStepIDs[pkg.Name],
			Name:     fmt.Sprintf("deploy-%s", pkg.Name),
			StepType: "deploy",
			Inputs: map[string]string{
				"package_name": pkg.Name,
				"package_type": pkg.PackageType,
			},
			Retry:    RetryPolicy{MaxRetries: 3, BaseDelayMs: 1000, MaxDelayMs: 30000},
			TimeoutS: 600,
		}

		for _, dep := range manifest.Dependencies {
			if dep.Required && dep.ProductID != pkg.Name {
				if depID, ok := pkgStepIDs[dep.ProductID]; ok {
					step.DependsOn = append(step.DependsOn, depID)
				}
			}
		}

		steps = append(steps, step)
	}

	artifactIndex := 0
	for _, art := range manifest.Artifacts {
		if artifactIndex < len(steps) {
			steps[artifactIndex].Inputs["artifact_digest"] = art.Digest
		}
		artifactIndex++
	}

	return steps
}

func (mb *ManifestBridge) artifactDigests(manifest *ReleaseManifest) []string {
	digests := make([]string, 0, len(manifest.Artifacts))
	for _, art := range manifest.Artifacts {
		digests = append(digests, art.Digest)
	}
	return digests
}
