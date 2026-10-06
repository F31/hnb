package worker

import (
	"strings"
	"testing"
)

func TestInstallArgsUsesManifestHelmConfig(t *testing.T) {
	w := New(nil, nil, nil, "helm", t.TempDir(), false)
	req := &Request{
		Name:         "prometheus-operator",
		Version:      "55.5.0",
		TargetID:     "target-a",
		Chart:        "kube-prometheus-stack",
		ChartVersion: "55.5.0",
		RepoName:     "prometheus-community",
		RepoURL:      "https://prometheus-community.github.io/helm-charts",
		Namespace:    "monitoring",
		Values: map[string]any{
			"grafana": map[string]any{"enabled": false},
		},
	}

	args, cleanup, err := w.installArgs(req)
	if err != nil {
		t.Fatal(err)
	}
	if cleanup == nil {
		t.Fatal("expected values cleanup")
	}
	defer cleanup()

	joined := strings.Join(args, " ")
	for _, want := range []string{
		"upgrade --install prometheus-operator prometheus-community/kube-prometheus-stack",
		"--namespace monitoring",
		"--version 55.5.0",
		"--values",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args %q missing %q", joined, want)
		}
	}
}

func TestInstallArgsSkipsVersionForLocalChart(t *testing.T) {
	w := New(nil, nil, nil, "helm", t.TempDir(), false)
	req := &Request{Name: "worker-e2e", Version: "0.1.0", TargetID: "target-a", Chart: "./charts/worker-e2e"}

	args, cleanup, err := w.installArgs(req)
	if err != nil {
		t.Fatal(err)
	}
	if cleanup != nil {
		defer cleanup()
	}
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "--version") {
		t.Fatalf("local chart args should not include --version: %q", joined)
	}
	if !strings.Contains(joined, "./charts/worker-e2e") {
		t.Fatalf("local chart missing from args: %q", joined)
	}
}

func TestInstallArgsFallsBackToBuiltInCatalog(t *testing.T) {
	w := New(nil, nil, nil, "helm", t.TempDir(), false)
	req := &Request{Name: "longhorn", Version: "1.7.2", TargetID: "target-a"}

	args, cleanup, err := w.installArgs(req)
	if err != nil {
		t.Fatal(err)
	}
	if cleanup != nil {
		defer cleanup()
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "longhorn/longhorn") || !strings.Contains(joined, "--namespace longhorn-system") {
		t.Fatalf("built-in catalog fallback missing: %q", joined)
	}
}

func TestServerDryRunArgsAppendsFlagsWithoutMutatingInstallArgs(t *testing.T) {
	installArgs := []string{"upgrade", "--install", "demo", "repo/demo"}
	dryRunArgs := serverDryRunArgs(installArgs)
	joined := strings.Join(dryRunArgs, " ")
	if !strings.Contains(joined, "--dry-run=server") || !strings.Contains(joined, "--debug") {
		t.Fatalf("server dry-run args missing flags: %q", joined)
	}
	if strings.Contains(strings.Join(installArgs, " "), "--dry-run=server") {
		t.Fatalf("install args mutated: %q", strings.Join(installArgs, " "))
	}
}

func TestStageResponses(t *testing.T) {
	stages := []StageEvent{{Name: "validate-request", Status: "succeeded"}}
	success := stageSuccess(stages)
	if success.Status != "succeeded" || success.Stage != "validate-request" || len(success.Stages) != 1 {
		t.Fatalf("success response = %+v", success)
	}
	failure := stageFailure(stages, "helm-install", "boom")
	if failure.Status != "failed" || failure.Stage != "helm-install" || !strings.Contains(failure.Message, "helm-install: boom") {
		t.Fatalf("failure response = %+v", failure)
	}
	if got := failure.Stages[len(failure.Stages)-1]; got.Name != "helm-install" || got.Status != "failed" || got.Message != "boom" {
		t.Fatalf("failure stage = %+v", got)
	}
}

func TestValidateHelmRequest(t *testing.T) {
	valid := &Request{
		Name:      "prometheus-operator",
		TargetID:  "515eba09-0a41-5b92-b972-69af1f0f655c",
		Chart:     "kube-prometheus-stack",
		RepoName:  "prometheus-community",
		RepoURL:   "https://prometheus-community.github.io/helm-charts",
		Namespace: "monitoring",
	}
	if err := validateHelmRequest(valid, true); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}

	for name, req := range map[string]*Request{
		"missing target": {Name: "demo", Chart: "demo"},
		"bad release":    {Name: "Bad_Name", TargetID: "target", Chart: "demo"},
		"bad namespace":  {Name: "demo", TargetID: "target", Chart: "demo", Namespace: "bad_namespace"},
		"bad repo url":   {Name: "demo", TargetID: "target", Chart: "demo", RepoURL: "https://user:pass@example.com/charts"},
		"bad chart":      {Name: "demo", TargetID: "target", Chart: "repo/chart/extra"},
		"bad version":    {Name: "demo", TargetID: "target", Chart: "demo", ChartVersion: "1.0.0 latest"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateHelmRequest(req, true); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestValidateHelmRequestRequiresExplicitOrKnownChart(t *testing.T) {
	if err := validateHelmRequest(&Request{Name: "kubeedge", TargetID: "target-a"}, true); err == nil || !strings.Contains(err.Error(), "chart is required") {
		t.Fatalf("expected chart required error, got %v", err)
	}
	if err := validateHelmRequest(&Request{Name: "longhorn", TargetID: "target-a"}, true); err != nil {
		t.Fatalf("known chart rejected: %v", err)
	}
}

func TestValidateHelmRequestAllowsLocalChart(t *testing.T) {
	req := &Request{Name: "worker-e2e", TargetID: "target-a", Chart: "./charts/worker-e2e"}
	if err := validateHelmRequest(req, true); err != nil {
		t.Fatalf("local chart rejected: %v", err)
	}
}

func TestValidateValuesSchema(t *testing.T) {
	values := map[string]any{
		"grafana":  map[string]any{"enabled": false},
		"replicas": float64(2),
	}
	schema := map[string]any{
		"required": []any{"grafana"},
		"properties": map[string]any{
			"grafana": map[string]any{
				"type":     "object",
				"required": []any{"enabled"},
				"properties": map[string]any{
					"enabled": map[string]any{"type": "boolean"},
				},
			},
			"replicas": map[string]any{"type": "integer"},
		},
	}
	if err := validateValuesSchema(values, schema); err != nil {
		t.Fatalf("valid values rejected: %v", err)
	}
}

func TestValidateValuesSchemaRejectsMissingRequired(t *testing.T) {
	schema := map[string]any{"required": []any{"grafana"}}
	if err := validateValuesSchema(map[string]any{}, schema); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("expected required error, got %v", err)
	}
}

func TestValidateValuesSchemaRejectsWrongType(t *testing.T) {
	values := map[string]any{"grafana": map[string]any{"enabled": "false"}}
	schema := map[string]any{"properties": map[string]any{
		"grafana": map[string]any{"type": "object", "properties": map[string]any{
			"enabled": map[string]any{"type": "boolean"},
		}},
	}}
	if err := validateValuesSchema(values, schema); err == nil || !strings.Contains(err.Error(), "boolean") {
		t.Fatalf("expected boolean type error, got %v", err)
	}
}

func TestValidateValuesSchemaExtendedConstraints(t *testing.T) {
	values := map[string]any{
		"mode":     "production",
		"replicas": float64(3),
		"image":    map[string]any{"tag": "v1.2.3"},
	}
	schema := map[string]any{
		"additionalProperties": false,
		"properties": map[string]any{
			"mode":     map[string]any{"type": "string", "enum": []any{"dev", "production"}},
			"replicas": map[string]any{"type": "integer", "minimum": float64(1), "maximum": float64(5)},
			"image": map[string]any{"type": "object", "properties": map[string]any{
				"tag": map[string]any{"type": "string", "minLength": float64(2), "maxLength": float64(16), "pattern": `^v\d+\.\d+\.\d+$`},
			}},
		},
	}
	if err := validateValuesSchema(values, schema); err != nil {
		t.Fatalf("valid extended values rejected: %v", err)
	}
}

func TestValidateValuesSchemaRejectsExtendedConstraintViolations(t *testing.T) {
	tests := map[string]struct {
		values map[string]any
		schema map[string]any
		want   string
	}{
		"enum": {
			values: map[string]any{"mode": "test"},
			schema: map[string]any{"properties": map[string]any{"mode": map[string]any{"enum": []any{"dev", "production"}}}},
			want:   "one of",
		},
		"minimum": {
			values: map[string]any{"replicas": float64(0)},
			schema: map[string]any{"properties": map[string]any{"replicas": map[string]any{"minimum": float64(1)}}},
			want:   ">= 1",
		},
		"maximum": {
			values: map[string]any{"replicas": float64(6)},
			schema: map[string]any{"properties": map[string]any{"replicas": map[string]any{"maximum": float64(5)}}},
			want:   "<= 5",
		},
		"minLength": {
			values: map[string]any{"tag": "v"},
			schema: map[string]any{"properties": map[string]any{"tag": map[string]any{"minLength": float64(2)}}},
			want:   ">= 2",
		},
		"maxLength": {
			values: map[string]any{"tag": "very-long-tag"},
			schema: map[string]any{"properties": map[string]any{"tag": map[string]any{"maxLength": float64(4)}}},
			want:   "<= 4",
		},
		"pattern": {
			values: map[string]any{"tag": "latest"},
			schema: map[string]any{"properties": map[string]any{"tag": map[string]any{"pattern": `^v\d+`}}},
			want:   "match",
		},
		"additionalProperties": {
			values: map[string]any{"unexpected": true},
			schema: map[string]any{"additionalProperties": false, "properties": map[string]any{}},
			want:   "not allowed",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			err := validateValuesSchema(tt.values, tt.schema)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q error, got %v", tt.want, err)
			}
		})
	}
}
