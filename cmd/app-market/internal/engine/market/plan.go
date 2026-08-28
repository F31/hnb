package market

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type ExecutionPlan struct {
	ID              string            `json:"id"`
	ReleaseID       string            `json:"release_id"`
	PlanDigest      string            `json:"plan_digest"`
	Steps           []StepSpec        `json:"steps"`
	ArtifactDigests []string          `json:"artifact_digests"`
	Outputs         map[string]string `json:"outputs"`
	PolicyResult    *PolicyResult     `json:"policy_result"`
	CreatedAt       string            `json:"created_at"`
}

type StepSpec struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	StepType  string            `json:"step_type"`
	DependsOn []string          `json:"depends_on"`
	Optional  bool              `json:"optional"`
	Inputs    map[string]string `json:"inputs"`
	Retry     RetryPolicy       `json:"retry"`
	TimeoutS  int               `json:"timeout_seconds"`
}

type RetryPolicy struct {
	MaxRetries  int `json:"max_retries"`
	BaseDelayMs int `json:"base_delay_ms"`
	MaxDelayMs  int `json:"max_delay_ms"`
}

type PolicyResult struct {
	Passed    bool              `json:"passed"`
	Policies  []string          `json:"policies"`
	Decisions map[string]string `json:"decisions"`
}

func computePlanDigest(plan *ExecutionPlan) (string, error) {
	data, err := json.Marshal(plan)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h[:]), nil
}

type planGenerator struct{}

func newPlanGenerator() *planGenerator {
	return &planGenerator{}
}

func (pg *planGenerator) generatePlan(
	releaseID string,
	steps []StepSpec,
	artifactDigests []string,
	policyResult *PolicyResult,
) (*ExecutionPlan, error) {
	sortedDigests := append([]string(nil), artifactDigests...)
	sort.Strings(sortedDigests)
	plan := &ExecutionPlan{
		ReleaseID:       releaseID,
		Steps:           steps,
		ArtifactDigests: sortedDigests,
		Outputs:         make(map[string]string),
		PolicyResult:    policyResult,
		CreatedAt:       "",
	}

	digest, err := computePlanDigest(plan)
	if err != nil {
		return nil, err
	}
	plan.PlanDigest = digest
	plan.ID = fmt.Sprintf("plan-%s", digest[:16])

	return plan, nil
}

func (pg *planGenerator) validatePlan(plan *ExecutionPlan) error {
	if len(plan.Steps) == 0 {
		return fmt.Errorf("plan must have at least one step")
	}
	resolver := newDAGResolver(plan.Steps)
	if _, err := resolver.resolve(); err != nil {
		return fmt.Errorf("invalid step DAG: %w", err)
	}
	if plan.PolicyResult != nil && !plan.PolicyResult.Passed {
		return fmt.Errorf("plan failed policy check: %v", plan.PolicyResult.Decisions)
	}
	for _, digest := range plan.ArtifactDigests {
		if !artifactDigestPattern.MatchString(digest) {
			return fmt.Errorf("plan artifact digest %q is not a verified sha256 reference", digest)
		}
	}
	return nil
}

type dagResolver struct {
	steps map[string]*StepSpec
	order []string
}

func newDAGResolver(steps []StepSpec) *dagResolver {
	s := make(map[string]*StepSpec, len(steps))
	for i := range steps {
		s[steps[i].ID] = &steps[i]
	}
	return &dagResolver{steps: s}
}

func (r *dagResolver) resolve() ([]string, error) {
	inDegree := make(map[string]int, len(r.steps))
	deps := make(map[string][]string, len(r.steps))

	for id, step := range r.steps {
		inDegree[id] = 0
		for _, dep := range step.DependsOn {
			if _, ok := r.steps[dep]; !ok {
				return nil, fmt.Errorf("step %q depends on unknown step %q", id, dep)
			}
			deps[dep] = append(deps[dep], id)
		}
	}

	for _, step := range r.steps {
		inDegree[step.ID] += len(step.DependsOn)
	}

	var queue []string
	for id, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, id)
		}
	}

	var order []string
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		order = append(order, current)

		for _, dependent := range deps[current] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}

	if len(order) != len(r.steps) {
		return nil, fmt.Errorf("cycle detected: resolved %d of %d steps", len(order), len(r.steps))
	}

	r.order = order
	return order, nil
}

func (r *dagResolver) executionLevels() ([][]string, error) {
	_, err := r.resolve()
	if err != nil {
		return nil, err
	}

	depth := make(map[string]int, len(r.steps))
	maxDepth := 0

	for _, id := range r.order {
		step := r.steps[id]
		d := 0
		for _, dep := range step.DependsOn {
			if depth[dep]+1 > d {
				d = depth[dep] + 1
			}
		}
		depth[id] = d
		if d > maxDepth {
			maxDepth = d
		}
	}

	levels := make([][]string, maxDepth+1)
	for id, d := range depth {
		levels[d] = append(levels[d], id)
	}

	return levels, nil
}

func OutputFormat(plan *ExecutionPlan) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Plan: %s (digest: %s)\n", plan.ID, plan.PlanDigest))
	b.WriteString(fmt.Sprintf("Steps: %d\n", len(plan.Steps)))
	for _, s := range plan.Steps {
		b.WriteString(fmt.Sprintf("  - %s [%s] type=%s\n", s.ID, s.Name, s.StepType))
		if len(s.DependsOn) > 0 {
			b.WriteString(fmt.Sprintf("    depends: %v\n", s.DependsOn))
		}
	}
	return b.String()
}
