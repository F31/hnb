package core

import "testing"

func TestIntentSemanticDigestCanonicalAndComplete(t *testing.T) {
	base := IntentSemanticDocument{APIVersion: "hnb.io/v1", Kind: "UpgradeRuntimeTarget", TargetID: "target-a", TargetKind: "KubernetesTarget", ExpectedVersion: 3, DesiredVersion: "v1.31", Parameters: map[string]any{"b": 2, "a": 1}}
	reordered := base
	reordered.Parameters = map[string]any{"a": 1, "b": 2}
	if IntentSemanticDigest(base) != IntentSemanticDigest(reordered) {
		t.Fatal("map order changed semantic digest")
	}
	changed := base
	changed.TargetID = "target-b"
	if IntentSemanticDigest(base) == IntentSemanticDigest(changed) {
		t.Fatal("target change did not alter semantic digest")
	}
}
