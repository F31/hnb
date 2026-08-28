package api

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/F31/hnb/cmd/platform-api/internal/store"
)

const (
	maxIdempotencyKeyLength = 128
	maxSteps                = 100
	maxInputValueLength     = 4096
	maxSecretRefLength      = 512
)

// sensitiveKeyPattern matches step input keys that would carry plaintext
// secrets. CFG-002: only secretReference strings are accepted, never values.
var sensitiveKeyPattern = regexp.MustCompile(`(?i)(password|passwd|secret|token|credential|private[_-]?key|api[_-]?key)`)

var privateKeyMaterialPattern = regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)

type validationError struct{ msg string }

func (e *validationError) Error() string { return e.msg }

func invalid(format string, args ...any) *validationError {
	return &validationError{msg: fmt.Sprintf(format, args...)}
}

func toSubmitCommand(req *submitOperationRequest) (store.SubmitCommand, error) {
	cmd := store.SubmitCommand{
		TenantID:         strings.TrimSpace(req.TenantID),
		ProjectID:        strings.TrimSpace(req.ProjectID),
		EnvironmentID:    strings.TrimSpace(req.EnvironmentID),
		NamespaceID:      strings.TrimSpace(req.NamespaceID),
		ReleaseID:        strings.TrimSpace(req.ReleaseID),
		OperationType:    strings.TrimSpace(req.OperationType),
		IdempotencyKey:   strings.TrimSpace(req.IdempotencyKey),
		InitiatedBy:      strings.TrimSpace(req.InitiatedBy),
		CorrelationID:    strings.TrimSpace(req.CorrelationID),
		TargetClusterIDs: req.TargetClusterIDs,
		Tags:             req.Tags,
	}
	if cmd.TenantID == "" {
		return cmd, invalid("tenantId is required")
	}
	if cmd.NamespaceID == "" {
		return cmd, invalid("namespaceId is required")
	}
	if cmd.ReleaseID == "" {
		return cmd, invalid("releaseId is required")
	}
	if cmd.InitiatedBy == "" {
		return cmd, invalid("initiatedBy is required")
	}
	if !store.IsValidOperationType(cmd.OperationType) {
		return cmd, invalid("operationType %q is not supported", req.OperationType)
	}
	if cmd.IdempotencyKey == "" {
		return cmd, invalid("idempotencyKey is required")
	}
	if len(cmd.IdempotencyKey) > maxIdempotencyKeyLength {
		return cmd, invalid("idempotencyKey exceeds %d characters", maxIdempotencyKeyLength)
	}
	if cmd.CorrelationID != "" {
		if _, err := uuid.Parse(cmd.CorrelationID); err != nil {
			return cmd, invalid("correlationId must be a UUID")
		}
	}
	if len(req.Steps) == 0 {
		return cmd, invalid("at least one step is required")
	}
	if len(req.Steps) > maxSteps {
		return cmd, invalid("at most %d steps are allowed", maxSteps)
	}

	seen := make(map[string]bool, len(req.Steps))
	for i := range req.Steps {
		step, err := toStepInput(&req.Steps[i], i)
		if err != nil {
			return cmd, err
		}
		if seen[step.PlanStepID] {
			return cmd, invalid("duplicate step id %q", step.PlanStepID)
		}
		seen[step.PlanStepID] = true
		key := store.StepIdempotencyKey(cmd.IdempotencyKey, step.PlanStepID)
		if len(key) > maxIdempotencyKeyLength {
			return cmd, invalid("derived step idempotency key for step %q exceeds %d characters", step.PlanStepID, maxIdempotencyKeyLength)
		}
		cmd.Steps = append(cmd.Steps, step)
	}
	if err := validateDependencies(cmd.Steps); err != nil {
		return cmd, err
	}
	if err := validateSchedulingPolicy(req.SchedulingPolicy); err != nil {
		return cmd, err
	}
	if req.SchedulingPolicy != nil {
		cmd.TargetClusterIDs = resolveClusterIDs(req.SchedulingPolicy)
	}
	return cmd, nil
}

func validateSchedulingPolicy(p *schedulingPolicy) error {
	if p == nil {
		return nil
	}
	switch p.Strategy {
	case "Duplicated", "Divide":
	default:
		return invalid("schedulingPolicy.strategy must be 'Duplicated' or 'Divide'")
	}
	if p.Selector != nil {
		if len(p.Selector.LabelSelector) == 0 && p.Selector.Region == "" && p.Selector.Zone == "" && len(p.Selector.ClusterIDs) == 0 {
			return invalid("schedulingPolicy.selector must specify at least one criterion")
		}
	}
	return nil
}

func resolveClusterIDs(p *schedulingPolicy) []string {
	if p == nil {
		return nil
	}
	if p.Selector != nil && len(p.Selector.ClusterIDs) > 0 {
		return p.Selector.ClusterIDs
	}
	return nil
}

func toStepInput(req *submitStepRequest, index int) (store.StepInput, error) {
	step := store.StepInput{
		PlanStepID:      strings.TrimSpace(req.ID),
		Name:            strings.TrimSpace(req.Name),
		StepType:        strings.TrimSpace(req.StepType),
		ProviderID:      strings.TrimSpace(req.ProviderID),
		DependsOn:       req.DependsOn,
		Optional:        req.Optional,
		Inputs:          req.Inputs,
		SecretReference: strings.TrimSpace(req.SecretReference),
		MaxRetries:      req.MaxRetries,
		TimeoutSeconds:  req.TimeoutSeconds,
	}
	if step.PlanStepID == "" {
		step.PlanStepID = fmt.Sprintf("step-%d", index+1)
	}
	if step.Name == "" {
		return step, invalid("step %q: name is required", step.PlanStepID)
	}
	if step.StepType == "" {
		return step, invalid("step %q: stepType is required", step.PlanStepID)
	}
	if step.TimeoutSeconds == 0 {
		step.TimeoutSeconds = 300
	}
	if step.TimeoutSeconds < 0 {
		return step, invalid("step %q: timeoutSeconds must be positive", step.PlanStepID)
	}
	if step.MaxRetries < 0 || step.MaxRetries > 9 {
		return step, invalid("step %q: maxRetries must be between 0 and 9", step.PlanStepID)
	}
	if step.MaxRetries == 0 {
		step.MaxRetries = 3
	}
	for key, value := range step.Inputs {
		if strings.EqualFold(key, "secretReference") {
			return step, invalid("step %q: use the secretReference field instead of an input named %q", step.PlanStepID, key)
		}
		if sensitiveKeyPattern.MatchString(key) && !strings.HasSuffix(strings.ToLower(key), "reference") {
			return step, invalid("step %q: input %q looks like a plaintext secret; pass a secretReference instead", step.PlanStepID, key)
		}
		if len(value) > maxInputValueLength {
			return step, invalid("step %q: input %q value exceeds %d characters", step.PlanStepID, key, maxInputValueLength)
		}
		if privateKeyMaterialPattern.MatchString(value) {
			return step, invalid("step %q: input %q contains private key material; pass a secretReference instead", step.PlanStepID, key)
		}
	}
	if step.SecretReference != "" {
		if len(step.SecretReference) > maxSecretRefLength {
			return step, invalid("step %q: secretReference exceeds %d characters", step.PlanStepID, maxSecretRefLength)
		}
		if strings.ContainsAny(step.SecretReference, " \t\n\r") {
			return step, invalid("step %q: secretReference must not contain whitespace", step.PlanStepID)
		}
	}
	return step, nil
}

// validateDependencies ensures depends_on references existing steps and the
// step graph is acyclic.
func validateDependencies(steps []store.StepInput) error {
	known := make(map[string]bool, len(steps))
	for _, step := range steps {
		known[step.PlanStepID] = true
	}
	edges := make(map[string][]string, len(steps))
	for _, step := range steps {
		for _, dep := range step.DependsOn {
			if !known[dep] {
				return invalid("step %q depends on unknown step %q", step.PlanStepID, dep)
			}
			if dep == step.PlanStepID {
				return invalid("step %q depends on itself", step.PlanStepID)
			}
			edges[step.PlanStepID] = append(edges[step.PlanStepID], dep)
		}
	}
	// Kahn's algorithm over the dependency graph.
	indegree := make(map[string]int, len(steps))
	for _, step := range steps {
		indegree[step.PlanStepID] = len(step.DependsOn)
	}
	queue := make([]string, 0, len(steps))
	for _, step := range steps {
		if indegree[step.PlanStepID] == 0 {
			queue = append(queue, step.PlanStepID)
		}
	}
	resolved := 0
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		resolved++
		for dependent, deps := range edges {
			for _, dep := range deps {
				if dep == current {
					indegree[dependent]--
					if indegree[dependent] == 0 {
						queue = append(queue, dependent)
					}
				}
			}
		}
	}
	if resolved != len(steps) {
		return invalid("step dependencies contain a cycle")
	}
	return nil
}

func validateAction(req *actionRequest) (tenantID, actorID string, err error) {
	tenantID = strings.TrimSpace(req.TenantID)
	actorID = strings.TrimSpace(req.ActorID)
	if tenantID == "" {
		return "", "", invalid("tenantId is required")
	}
	if actorID == "" {
		return "", "", invalid("actorId is required")
	}
	return tenantID, actorID, nil
}
