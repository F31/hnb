package appstore

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestUploadSessionReleaseIDJSON(t *testing.T) {
	releaseID := "release-a"
	data, err := json.Marshal(UploadSession{ID: "session-a", ReleaseID: &releaseID})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"release_id":"release-a"`) {
		t.Fatalf("release_id missing from upload session JSON: %s", data)
	}

	var session UploadSession
	if err := json.Unmarshal([]byte(`{"release_id":"release-b"}`), &session); err != nil {
		t.Fatal(err)
	}
	if session.ReleaseID == nil || *session.ReleaseID != "release-b" {
		t.Fatalf("release_id did not round trip: %+v", session.ReleaseID)
	}
}
