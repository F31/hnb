package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRetainedVolumeContractsExcludeGenericClaimRefSanitization(t *testing.T) {
	root := retainedVolumeContractRoot(t)
	for _, name := range []string{"retained-volume-workflow-step-input.schema.json", "retained-volume-workflow-result.schema.json"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		var document map[string]any
		if json.Unmarshal(data, &document) != nil {
			t.Fatalf("%s is not valid JSON", name)
		}
		if strings.Contains(strings.ToLower(string(data)), "claimref") {
			t.Fatalf("%s permits generic claimRef sanitization", name)
		}
	}
}

func TestSanitizedResultContractRequiresProviderEvidence(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(retainedVolumeContractRoot(t), "retained-volume-workflow-result.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, required := range []string{`"state": { "const": "Sanitized" }`, `"required": ["sanitizationEvidence"]`, `"evidenceDigest"`, `"fencingGeneration"`, `"ManualReleaseRequired"`, `"dataRetained": { "const": true }`} {
		if !strings.Contains(text, required) {
			t.Fatalf("result contract lacks %s", required)
		}
	}
}

func retainedVolumeContractRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate contract test")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../../../contracts/schema/storage/v1"))
}
