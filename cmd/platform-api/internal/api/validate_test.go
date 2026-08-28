package api

import (
	"strings"
	"testing"

	"github.com/F31/hnb/cmd/platform-api/internal/store"
)

func validSubmitRequest() *submitOperationRequest {
	return &submitOperationRequest{
		TenantID:       "tenant-a",
		NamespaceID:    "ns-prod",
		ReleaseID:      "rel-1",
		OperationType:  "deploy",
		IdempotencyKey: "submit-1",
		InitiatedBy:    "user-1",
		Steps: []submitStepRequest{
			{ID: "build", Name: "build", StepType: "http"},
			{ID: "rollout", Name: "rollout", StepType: "http", DependsOn: []string{"build"}},
		},
	}
}

func TestToSubmitCommandValid(t *testing.T) {
	cmd, err := toSubmitCommand(validSubmitRequest())
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if cmd.Steps[0].PlanStepID != "build" || cmd.Steps[1].PlanStepID != "rollout" {
		t.Fatalf("plan step ids = %q, %q", cmd.Steps[0].PlanStepID, cmd.Steps[1].PlanStepID)
	}
	if cmd.Steps[0].TimeoutSeconds != 300 || cmd.Steps[0].MaxRetries != 3 {
		t.Fatalf("defaults not applied: %+v", cmd.Steps[0])
	}
}

func TestToSubmitCommandMissingFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(r *submitOperationRequest)
		part   string
	}{
		{"tenant", func(r *submitOperationRequest) { r.TenantID = "" }, "tenantId"},
		{"namespace", func(r *submitOperationRequest) { r.NamespaceID = " " }, "namespaceId"},
		{"release", func(r *submitOperationRequest) { r.ReleaseID = "" }, "releaseId"},
		{"initiator", func(r *submitOperationRequest) { r.InitiatedBy = "" }, "initiatedBy"},
		{"type", func(r *submitOperationRequest) { r.OperationType = "exec" }, "operationType"},
		{"idempotency", func(r *submitOperationRequest) { r.IdempotencyKey = "" }, "idempotencyKey"},
		{"idempotency too long", func(r *submitOperationRequest) { r.IdempotencyKey = strings.Repeat("k", 129) }, "idempotencyKey"},
		{"no steps", func(r *submitOperationRequest) { r.Steps = nil }, "at least one step"},
		{"bad correlation", func(r *submitOperationRequest) { r.CorrelationID = "not-a-uuid" }, "correlationId"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validSubmitRequest()
			tt.mutate(req)
			_, err := toSubmitCommand(req)
			if err == nil || !strings.Contains(err.Error(), tt.part) {
				t.Fatalf("error = %v, want containing %q", err, tt.part)
			}
		})
	}
}

func TestToSubmitCommandRejectsPlaintextSecrets(t *testing.T) {
	tests := []struct {
		name   string
		inputs map[string]string
	}{
		{"password key", map[string]string{"dbPassword": "p@ssw0rd"}},
		{"token key", map[string]string{"api_token": "tok"}},
		{"secret key", map[string]string{"client_secret": "abc"}},
		{"private key material", map[string]string{"cert": "-----BEGIN RSA PRIVATE KEY-----\n..."}},
		{"reserved input name", map[string]string{"secretReference": "secret://ns/name"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validSubmitRequest()
			req.Steps[0].Inputs = tt.inputs
			_, err := toSubmitCommand(req)
			if err == nil || !strings.Contains(err.Error(), "secret") && !strings.Contains(err.Error(), "private key") {
				t.Fatalf("error = %v, want a secret-related rejection", err)
			}
		})
	}
}

func TestToSubmitCommandAcceptsSecretReference(t *testing.T) {
	req := validSubmitRequest()
	req.Steps[0].SecretReference = "secret://prod/db-credentials"
	req.Steps[0].Inputs = map[string]string{"passwordReference": "secret://prod/db-credentials"}
	cmd, err := toSubmitCommand(req)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if cmd.Steps[0].SecretReference != "secret://prod/db-credentials" {
		t.Fatalf("secretReference = %q", cmd.Steps[0].SecretReference)
	}
}

func TestValidateDependencies(t *testing.T) {
	tests := []struct {
		name  string
		steps []store.StepInput
		part  string
	}{
		{
			name: "unknown dependency",
			steps: []store.StepInput{
				{PlanStepID: "a", DependsOn: []string{"ghost"}},
			},
			part: "unknown step",
		},
		{
			name: "self dependency",
			steps: []store.StepInput{
				{PlanStepID: "a", DependsOn: []string{"a"}},
			},
			part: "itself",
		},
		{
			name: "cycle",
			steps: []store.StepInput{
				{PlanStepID: "a", DependsOn: []string{"b"}},
				{PlanStepID: "b", DependsOn: []string{"a"}},
			},
			part: "cycle",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDependencies(tt.steps)
			if err == nil || !strings.Contains(err.Error(), tt.part) {
				t.Fatalf("error = %v, want containing %q", err, tt.part)
			}
		})
	}
}

func TestValidateDependenciesDAG(t *testing.T) {
	steps := []store.StepInput{
		{PlanStepID: "a"},
		{PlanStepID: "b", DependsOn: []string{"a"}},
		{PlanStepID: "c", DependsOn: []string{"a", "b"}},
	}
	if err := validateDependencies(steps); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestDerivedStepKeyLength(t *testing.T) {
	req := validSubmitRequest()
	req.IdempotencyKey = strings.Repeat("k", 121)
	_, err := toSubmitCommand(req)
	if err == nil || !strings.Contains(err.Error(), "idempotency key") {
		t.Fatalf("error = %v, want derived key length rejection", err)
	}
}

func TestToSubmitCommandWithSchedulingPolicy(t *testing.T) {
	req := validSubmitRequest()
	req.SchedulingPolicy = &schedulingPolicy{
		Strategy: "Duplicated",
		Selector: &clusterSelector{
			Region: "cn-east",
		},
	}
	cmd, err := toSubmitCommand(req)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if len(cmd.TargetClusterIDs) != 0 {
		t.Fatalf("TargetClusterIDs = %v, want empty (no explicit cluster IDs)", cmd.TargetClusterIDs)
	}
}

func TestToSubmitCommandWithExplicitClusterIDs(t *testing.T) {
	req := validSubmitRequest()
	req.SchedulingPolicy = &schedulingPolicy{
		Strategy: "Divide",
		Selector: &clusterSelector{
			ClusterIDs: []string{"cluster-a", "cluster-b"},
		},
	}
	cmd, err := toSubmitCommand(req)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if len(cmd.TargetClusterIDs) != 2 || cmd.TargetClusterIDs[0] != "cluster-a" {
		t.Fatalf("TargetClusterIDs = %v, want [cluster-a cluster-b]", cmd.TargetClusterIDs)
	}
}

func TestToSubmitCommandInvalidSchedulingPolicy(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(r *submitOperationRequest)
		part   string
	}{
		{"bad strategy", func(r *submitOperationRequest) { r.SchedulingPolicy.Strategy = "Unknown" }, "strategy"},
		{"empty selector", func(r *submitOperationRequest) {
			r.SchedulingPolicy.Selector = &clusterSelector{}
		}, "at least one criterion"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validSubmitRequest()
			req.SchedulingPolicy = &schedulingPolicy{Strategy: "Duplicated"}
			tt.mutate(req)
			_, err := toSubmitCommand(req)
			if err == nil || !strings.Contains(err.Error(), tt.part) {
				t.Fatalf("error = %v, want containing %q", err, tt.part)
			}
		})
	}
}

func TestToSubmitCommandTargetClusterIDsDirect(t *testing.T) {
	req := validSubmitRequest()
	req.TargetClusterIDs = []string{"cluster-a", "cluster-b"}
	cmd, err := toSubmitCommand(req)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if len(cmd.TargetClusterIDs) != 2 || cmd.TargetClusterIDs[0] != "cluster-a" {
		t.Fatalf("TargetClusterIDs = %v, want [cluster-a cluster-b]", cmd.TargetClusterIDs)
	}
}
