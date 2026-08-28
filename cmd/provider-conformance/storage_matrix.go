package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const storageSuiteVersion = "1.0.0"

var storageSemverPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

type StorageMatrix struct {
	SchemaVersion           string              `json:"schemaVersion"`
	MatrixVersion           string              `json:"matrixVersion"`
	SuiteVersion            string              `json:"suiteVersion"`
	HNBCoreVersion          string              `json:"hnbCoreVersion"`
	ProviderProtocolVersion string              `json:"providerProtocolVersion"`
	Tiers                   []StorageMatrixTier `json:"tiers"`
}

type StorageMatrixTier struct {
	ID           string            `json:"id"`
	InitialTier  string            `json:"initialTier"`
	Capabilities map[string]string `json:"capabilities"`
	Lifecycle    map[string]string `json:"lifecycle"`
}

type StorageEvidence struct {
	SchemaVersion           string                `json:"schemaVersion"`
	MatrixVersion           string                `json:"matrixVersion"`
	SuiteVersion            string                `json:"suiteVersion"`
	HNBCoreVersion          string                `json:"hnbCoreVersion"`
	ProviderProtocolVersion string                `json:"providerProtocolVersion"`
	Tiers                   []StorageEvidenceTier `json:"tiers"`
}

type StorageEvidenceTier struct {
	ID                       string                    `json:"id"`
	PackageID                string                    `json:"packageId"`
	PackageVersion           string                    `json:"packageVersion"`
	KubernetesVersion        string                    `json:"kubernetesVersion"`
	KubernetesCompatibility  []VersionRange            `json:"kubernetesCompatibility"`
	PackageClaims            map[string]string         `json:"packageClaims"`
	ObservedCapabilities     map[string]CapabilityFact `json:"observedCapabilities"`
	Lifecycle                map[string]LifecycleFact  `json:"lifecycle"`
	PersistedArtifacts       []map[string]any          `json:"persistedArtifacts"`
	ProviderImplemented      bool                      `json:"providerImplemented"`
	ProductionReadyRequested bool                      `json:"productionReadyRequested"`
}

type VersionRange struct {
	MinInclusive string `json:"minInclusive"`
	MaxExclusive string `json:"maxExclusive"`
}

type CapabilityFact struct {
	Status      string `json:"status"`
	EvidenceRef string `json:"evidenceRef"`
}

type LifecycleFact struct {
	Status             string `json:"status"`
	PlannerProviderID  string `json:"plannerProviderId"`
	ExecutedProviderID string `json:"executedProviderId"`
	StepType           string `json:"stepType"`
	IdempotentReplay   bool   `json:"idempotentReplay"`
	StaleFenceRejected bool   `json:"staleFenceRejected"`
	RollbackMetadata   bool   `json:"rollbackMetadata"`
	EvidenceRef        string `json:"evidenceRef"`
}

func RunStorageMatrix(matrixPath, evidencePath string) ([]TestResult, error) {
	var matrix StorageMatrix
	if err := readJSON(matrixPath, &matrix); err != nil {
		return nil, fmt.Errorf("read storage matrix: %w", err)
	}
	var evidence StorageEvidence
	if err := readJSON(evidencePath, &evidence); err != nil {
		return nil, fmt.Errorf("read storage evidence: %w", err)
	}
	return validateStorageMatrix(matrix, evidence), nil
}

func readJSON(path string, target any) error {
	document, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(document)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return nil
}

func validateStorageMatrix(matrix StorageMatrix, evidence StorageEvidence) []TestResult {
	results := []TestResult{}
	add := func(name, category string, err error) {
		result := TestResult{TestName: name, Category: category, Passed: err == nil, Duration: "0s"}
		if err != nil {
			result.Error = err.Error()
		}
		results = append(results, result)
	}

	add("storage/version-binding", "storage-contract", validateVersionBinding(matrix, evidence))
	matrixTiers := make(map[string]StorageMatrixTier, len(matrix.Tiers))
	for _, tier := range matrix.Tiers {
		matrixTiers[tier.ID] = tier
	}
	evidenceTiers := make(map[string]StorageEvidenceTier, len(evidence.Tiers))
	for _, tier := range evidence.Tiers {
		evidenceTiers[tier.ID] = tier
	}
	for _, id := range []string{"generic-csi", "nfs-static-pv", "ceph", "cloud-disk", "local-pv"} {
		matrixTier, matrixOK := matrixTiers[id]
		evidenceTier, evidenceOK := evidenceTiers[id]
		if !matrixOK || !evidenceOK {
			add("storage/"+id+"/fixture", "storage-contract", fmt.Errorf("matrix and evidence are required"))
			continue
		}
		add("storage/"+id+"/kubernetes-compatibility", "storage-compatibility", validateKubernetesCompatibility(evidenceTier))
		capabilities := sortedKeys(matrixTier.Capabilities)
		for _, capability := range capabilities {
			add("storage/"+id+"/capability/"+capability, "storage-capability", validateCapabilityCell(matrixTier, evidenceTier, capability))
		}
		for _, action := range []string{"install", "upgrade", "uninstall"} {
			add("storage/"+id+"/lifecycle/"+action, "storage-lifecycle", validateLifecycleCell(matrixTier, evidenceTier, action))
		}
		add("storage/"+id+"/secret-leakage", "storage-security", validateNoSecretLeakage(evidenceTier.PersistedArtifacts))
		add("storage/"+id+"/production-readiness", "storage-production-gate", validateProductionReadiness(matrixTier, evidenceTier))
	}
	return results
}

func validateVersionBinding(matrix StorageMatrix, evidence StorageEvidence) error {
	if matrix.SchemaVersion != "1.0.0" || evidence.SchemaVersion != "1.0.0" || matrix.SuiteVersion != storageSuiteVersion {
		return fmt.Errorf("schemaVersion and suiteVersion must be 1.0.0")
	}
	if matrix.MatrixVersion == "" || matrix.HNBCoreVersion == "" || matrix.ProviderProtocolVersion == "" {
		return fmt.Errorf("matrix, HNB Core and provider protocol versions are required")
	}
	if evidence.MatrixVersion != matrix.MatrixVersion || evidence.SuiteVersion != matrix.SuiteVersion || evidence.HNBCoreVersion != matrix.HNBCoreVersion || evidence.ProviderProtocolVersion != matrix.ProviderProtocolVersion {
		return fmt.Errorf("evidence is not bound to the exact matrix, suite, HNB Core and provider protocol versions")
	}
	return nil
}

func validateKubernetesCompatibility(tier StorageEvidenceTier) error {
	version, ok := parseSemver(tier.KubernetesVersion)
	if !ok || tier.PackageVersion == "" || len(tier.KubernetesCompatibility) == 0 {
		return fmt.Errorf("package and tested Kubernetes versions are required")
	}
	for _, candidate := range tier.KubernetesCompatibility {
		min, minOK := parseSemver(candidate.MinInclusive)
		max, maxOK := parseSemver(candidate.MaxExclusive)
		if minOK && maxOK && compareVersion(version, min) >= 0 && compareVersion(version, max) < 0 {
			return nil
		}
	}
	return fmt.Errorf("tested Kubernetes version %q is outside package compatibility", tier.KubernetesVersion)
}

func validateCapabilityCell(matrix StorageMatrixTier, evidence StorageEvidenceTier, capability string) error {
	want := matrix.Capabilities[capability]
	claim, claimOK := evidence.PackageClaims[capability]
	fact, factOK := evidence.ObservedCapabilities[capability]
	if want != "Supported" && want != "Unsupported" {
		return fmt.Errorf("invalid matrix status %q", want)
	}
	if !claimOK || claim != want {
		return fmt.Errorf("package claim %q does not match matrix %q", claim, want)
	}
	if !factOK || fact.Status != want || fact.EvidenceRef == "" {
		return fmt.Errorf("observed evidence does not prove %s", want)
	}
	return nil
}

func validateLifecycleCell(matrix StorageMatrixTier, evidence StorageEvidenceTier, action string) error {
	want, matrixOK := matrix.Lifecycle[action]
	fact, evidenceOK := evidence.Lifecycle[action]
	if !matrixOK || !evidenceOK || fact.EvidenceRef == "" {
		return fmt.Errorf("lifecycle matrix cell and evidence are required")
	}
	if fact.Status != want {
		return fmt.Errorf("observed status %q does not match matrix %q", fact.Status, want)
	}
	if want == "Unsupported" {
		if fact.PlannerProviderID != "" || fact.ExecutedProviderID != "" || fact.StepType != "" {
			return fmt.Errorf("unsupported action was routed")
		}
		return nil
	}
	if want != "Supported" || fact.PlannerProviderID != evidence.PackageID || fact.ExecutedProviderID != evidence.PackageID || fact.StepType != "storage.driver."+action {
		return fmt.Errorf("planner/provider routing is not pinned to the package")
	}
	if !fact.IdempotentReplay || !fact.StaleFenceRejected {
		return fmt.Errorf("idempotent replay and stale fencing rejection are required")
	}
	if (action == "install" || action == "upgrade") && !fact.RollbackMetadata {
		return fmt.Errorf("rollback metadata is required")
	}
	return nil
}

func validateNoSecretLeakage(artifacts []map[string]any) error {
	for _, artifact := range artifacts {
		if path := secretValuePath(artifact, "artifact"); path != "" {
			return fmt.Errorf("inline Secret material at %s", path)
		}
	}
	return nil
}

func secretValuePath(value any, path string) string {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "_", ""), "-", ""))
			if normalized == "secretvalue" || normalized == "password" || normalized == "token" || normalized == "kubeconfig" || normalized == "privatekey" {
				return path + "." + key
			}
			if found := secretValuePath(child, path+"."+key); found != "" {
				return found
			}
		}
	case []any:
		for index, child := range typed {
			if found := secretValuePath(child, fmt.Sprintf("%s[%d]", path, index)); found != "" {
				return found
			}
		}
	}
	return ""
}

func validateProductionReadiness(matrix StorageMatrixTier, evidence StorageEvidenceTier) error {
	if evidence.ProductionReadyRequested && !evidence.ProviderImplemented {
		return fmt.Errorf("Production Ready fails closed without an implemented provider")
	}
	if evidence.ProductionReadyRequested && matrix.InitialTier != "T1" {
		return fmt.Errorf("%s tier is not eligible for initial Production Ready", matrix.InitialTier)
	}
	return nil
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func parseSemver(raw string) ([3]int, bool) {
	var result [3]int
	raw = strings.TrimPrefix(raw, "v")
	if !storageSemverPattern.MatchString(raw) {
		return result, false
	}
	for index, part := range strings.Split(raw, ".") {
		value, err := strconv.Atoi(part)
		if err != nil {
			return result, false
		}
		result[index] = value
	}
	return result, true
}

func compareVersion(left, right [3]int) int {
	for index := range left {
		if left[index] < right[index] {
			return -1
		}
		if left[index] > right[index] {
			return 1
		}
	}
	return 0
}
