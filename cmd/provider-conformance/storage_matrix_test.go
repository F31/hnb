package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadStorageFixtures(t *testing.T) (StorageMatrix, StorageEvidence) {
	t.Helper()
	var matrix StorageMatrix
	var evidence StorageEvidence
	if err := readJSON(filepath.Join("testdata", "storage-matrix.v1.json"), &matrix); err != nil {
		t.Fatal(err)
	}
	if err := readJSON(filepath.Join("testdata", "storage-evidence.v1.json"), &evidence); err != nil {
		t.Fatal(err)
	}
	return matrix, evidence
}

func TestStorageConformanceMatrix(t *testing.T) {
	results, err := RunStorageMatrix(filepath.Join("testdata", "storage-matrix.v1.json"), filepath.Join("testdata", "storage-evidence.v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(results) < 60 {
		t.Fatalf("matrix coverage too small: %d", len(results))
	}
	for _, result := range results {
		if !result.Passed {
			t.Errorf("%s: %s", result.TestName, result.Error)
		}
	}
}

func TestStorageConformanceFailsClosed(t *testing.T) {
	matrix, evidence := loadStorageFixtures(t)
	tests := []struct {
		name   string
		mutate func(*StorageEvidence)
		want   string
	}{
		{"claim exceeds observation", func(e *StorageEvidence) {
			e.Tiers[0].ObservedCapabilities["snapshot"] = CapabilityFact{Status: "Unsupported", EvidenceRef: "evidence://observed"}
		}, "capability/snapshot"},
		{"unsupported action routed", func(e *StorageEvidence) {
			fact := e.Tiers[1].Lifecycle["upgrade"]
			fact.PlannerProviderID = e.Tiers[1].PackageID
			e.Tiers[1].Lifecycle["upgrade"] = fact
		}, "lifecycle/upgrade"},
		{"Kubernetes outside range", func(e *StorageEvidence) { e.Tiers[2].KubernetesVersion = "1.35.0" }, "kubernetes-compatibility"},
		{"malformed Kubernetes version", func(e *StorageEvidence) { e.Tiers[2].KubernetesVersion = "1.32.0-dev" }, "kubernetes-compatibility"},
		{"missing rollback metadata", func(e *StorageEvidence) {
			fact := e.Tiers[3].Lifecycle["upgrade"]
			fact.RollbackMetadata = false
			e.Tiers[3].Lifecycle["upgrade"] = fact
		}, "lifecycle/upgrade"},
		{"secret leaked", func(e *StorageEvidence) { e.Tiers[4].PersistedArtifacts[0]["token"] = "plaintext" }, "secret-leakage"},
		{"fixture cannot certify provider", func(e *StorageEvidence) { e.Tiers[0].ProductionReadyRequested = true }, "production-readiness"},
		{"version mismatch", func(e *StorageEvidence) { e.MatrixVersion = "2.0.0" }, "version-binding"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			copyBytes, _ := json.Marshal(evidence)
			var changed StorageEvidence
			_ = json.Unmarshal(copyBytes, &changed)
			test.mutate(&changed)
			results := validateStorageMatrix(matrix, changed)
			for _, result := range results {
				if strings.Contains(result.TestName, test.want) && !result.Passed {
					return
				}
			}
			t.Fatalf("expected failing %s cell", test.want)
		})
	}
}

func TestStorageMatrixDecoderRejectsUnknownContractFields(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "matrix.json")
	if err := os.WriteFile(path, []byte(`{"schemaVersion":"1.0.0","unknown":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var matrix StorageMatrix
	if err := readJSON(path, &matrix); err == nil {
		t.Fatal("unknown matrix field was accepted")
	}
}
