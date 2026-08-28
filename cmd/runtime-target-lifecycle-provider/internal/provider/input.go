package provider

import (
	"fmt"
	"net/url"
	"strconv"

	"github.com/google/uuid"
)

func parseLifecycleInput(profile Profile, execution ExecutionContext) (LifecycleInput, error) {
	if execution.TenantID == "" || execution.OperationID == "" || execution.StepID == "" || execution.IdempotencyKey == "" {
		return LifecycleInput{}, invalid("tenant, operation, step, and idempotency key are required")
	}
	parsedAttempt, err := uuid.Parse(execution.ExecutionAttemptID)
	if err != nil || parsedAttempt == uuid.Nil || parsedAttempt.String() != execution.ExecutionAttemptID {
		return LifecycleInput{}, invalid("execution_attempt_id must be a canonical UUID")
	}
	action, ok := profile.StepActions[execution.StepType]
	if !ok {
		return LifecycleInput{}, fail(400, ErrorUnsupportedAction, false, "unsupported step type %q", execution.StepType)
	}
	if err := validateInputKeys(execution.Inputs, allowedKeys(profile.TargetKind)...); err != nil {
		return LifecycleInput{}, err
	}
	input := LifecycleInput{
		SchemaVersion:      stringInput(execution.Inputs, "schemaVersion"),
		TargetID:           stringInput(execution.Inputs, "targetId"),
		TargetKind:         stringInput(execution.Inputs, "targetKind"),
		Action:             stringInput(execution.Inputs, "action"),
		DisplayName:        stringInput(execution.Inputs, "displayName"),
		DesiredVersion:     stringInput(execution.Inputs, "desiredVersion"),
		CloudCoreEndpoint:  stringInput(execution.Inputs, "cloudCoreEndpoint"),
		IdempotencyKey:     stringInput(execution.Inputs, "idempotencyKey"),
		FencingGeneration:  intInput(execution.Inputs, "fencingGeneration"),
		ObservationVersion: intInput(execution.Inputs, "observationVersion"),
	}
	if input.SchemaVersion != "1.0.0" {
		return LifecycleInput{}, invalid("inputs.schemaVersion must be 1.0.0")
	}
	if parsedTarget, err := uuid.Parse(input.TargetID); err != nil || parsedTarget == uuid.Nil || parsedTarget.String() != input.TargetID {
		return LifecycleInput{}, invalid("inputs.targetId must be a canonical UUID")
	}
	if input.TargetKind != profile.TargetKind {
		return LifecycleInput{}, invalid("inputs.targetKind %q does not match %q", input.TargetKind, profile.TargetKind)
	}
	if input.Action != action {
		return LifecycleInput{}, invalid("step type %q requires action %q", execution.StepType, action)
	}
	if input.IdempotencyKey != execution.IdempotencyKey {
		return LifecycleInput{}, invalid("inputs.idempotencyKey must match execution idempotency_key")
	}
	if input.FencingGeneration != execution.FencingGeneration {
		return LifecycleInput{}, invalid("inputs.fencingGeneration must match execution fencing_generation")
	}
	if input.ObservationVersion < 0 {
		return LifecycleInput{}, invalid("inputs.observationVersion must be non-negative")
	}
	ref, err := secretRefInput(execution.Inputs, "credentialSecretRef")
	if err != nil {
		return LifecycleInput{}, err
	}
	input.CredentialSecretRef = ref
	if action == "create" || action == "import" {
		if input.DisplayName == "" || len(input.DisplayName) > 256 {
			return LifecycleInput{}, invalid("inputs.displayName is required and must be <= 256 characters")
		}
		if input.CredentialSecretRef == nil {
			return LifecycleInput{}, invalid("inputs.credentialSecretRef is required for %s", action)
		}
	}
	if action == "upgrade" && (input.DesiredVersion == "" || len(input.DesiredVersion) > 128) {
		return LifecycleInput{}, invalid("inputs.desiredVersion is required for upgrade")
	}
	if profile.TargetKind == "EdgeRuntimeTarget" && action == "import" {
		if err := validateCloudCoreEndpoint(input.CloudCoreEndpoint); err != nil {
			return LifecycleInput{}, err
		}
	}
	return input, nil
}

func allowedKeys(targetKind string) []string {
	keys := []string{"schemaVersion", "targetId", "targetKind", "action", "displayName", "desiredVersion", "credentialSecretRef", "idempotencyKey", "fencingGeneration", "observationVersion"}
	if targetKind == "EdgeRuntimeTarget" {
		keys = append(keys, "cloudCoreEndpoint", "nodeGroupMappings")
	}
	return keys
}

func validateInputKeys(inputs map[string]any, allowed ...string) error {
	valid := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		valid[key] = struct{}{}
	}
	for key := range inputs {
		if _, ok := valid[key]; !ok {
			return invalid("unsupported input %q", key)
		}
	}
	return nil
}

func stringInput(inputs map[string]any, key string) string {
	value, ok := inputs[key]
	if !ok || value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprintf("%v", value)
}

func intInput(inputs map[string]any, key string) int64 {
	value, ok := inputs[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case float64:
		return int64(typed)
	case int64:
		return typed
	case int:
		return int64(typed)
	case string:
		parsed, _ := strconv.ParseInt(typed, 10, 64)
		return parsed
	default:
		return 0
	}
}

func secretRefInput(inputs map[string]any, key string) (*SecretReference, error) {
	value, ok := inputs[key]
	if !ok || value == nil {
		return nil, nil
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, invalid("inputs.%s must be an object", key)
	}
	ref := SecretReference{
		Provider: stringInput(object, "provider"),
		Scope:    stringInput(object, "scope"),
		Name:     stringInput(object, "name"),
		Version:  stringInput(object, "version"),
	}
	if ref.Provider == "" || ref.Scope == "" || ref.Name == "" || ref.Version == "" {
		return nil, invalid("inputs.%s must include provider, scope, name, and version", key)
	}
	return &ref, nil
}

func validateCloudCoreEndpoint(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "wss") {
		return invalid("inputs.cloudCoreEndpoint must be an https or wss URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return invalid("inputs.cloudCoreEndpoint must not include userinfo, query, or fragment")
	}
	return nil
}
