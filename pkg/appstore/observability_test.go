package appstore

import "testing"

func TestArtifactEventRedactionAvoidsSecretWords(t *testing.T) {
	event := redactArtifactEvent(ArtifactEvent{Event: "robot_token_cleanup", TenantID: "tenant-a", ArtifactID: "artifact-a"})
	if event.Event != "redacted" {
		t.Fatalf("secret-like event was not redacted: %+v", event)
	}
}
