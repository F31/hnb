package toolkit

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// releaseStoreFixture is a live TLS platform recording one /api/v1/releases call.
type releaseStoreFixture struct {
	captured map[string]any
	status   int
	body     string
}

func newReleaseStoreFixture(status int, body string) (*httptest.Server, *releaseStoreFixture) {
	f := &releaseStoreFixture{status: status, body: body}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer fixture-api-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		raw, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		_ = json.Unmarshal(raw, &f.captured)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(f.status)
		_, _ = w.Write([]byte(f.body))
	}))
	return server, f
}

func validReleaseEntry() ReleaseMaterialEntry {
	return ReleaseMaterialEntry{
		ReleaseID: "r1", ReleaseVersion: "1.0", Role: string(RoleWorker),
		BundleID: "bundle-1111111111111111", LockDigest: "sha256:" + strings.Repeat("1", 64),
		ContentDigest: "sha256:" + strings.Repeat("2", 64),
	}
}

func releaseStoreOKBody() string {
	return `{"id":"op-1","releaseId":"r1","releaseVersion":"1.0","role":"worker","bundleId":"bundle-1111111111111111","lockDigest":"` +
		strings.Repeat("1", 64) + `","contentDigest":"` + strings.Repeat("2", 64) + `"}`
}

func TestRegisterReleaseMaterialMapsStatus(t *testing.T) {
	server, f := newReleaseStoreFixture(http.StatusCreated, releaseStoreOKBody())
	client := &PlatformClient{BaseURL: server.URL, Token: "fixture-api-token", Client: server.Client()}

	entry := validReleaseEntry()
	got, created, err := client.RegisterReleaseMaterial(context.Background(), entry)
	if err != nil || !created {
		t.Fatalf("created register failed: %+v %v", got, err)
	}
	if f.captured["role"] != "worker" || f.captured["releaseId"] != "r1" {
		t.Fatalf("captured body = %v", f.captured)
	}

	server2, _ := newReleaseStoreFixture(http.StatusOK, releaseStoreOKBody())
	client2 := &PlatformClient{BaseURL: server2.URL, Token: "fixture-api-token", Client: server2.Client()}
	if _, created, err := client2.RegisterReleaseMaterial(context.Background(), entry); err != nil || created {
		t.Fatalf("duplicate register was not idempotent: %v", err)
	}
}

func TestRegisterReleaseMaterialMapsRejectionAndAvailability(t *testing.T) {
	server, _ := newReleaseStoreFixture(http.StatusConflict, `{"code":"CONFLICT","message":"fixed release"}`)
	client := &PlatformClient{BaseURL: server.URL, Token: "fixture-api-token", Client: server.Client()}
	if _, _, err := client.RegisterReleaseMaterial(context.Background(), validReleaseEntry()); err == nil || !strings.Contains(err.Error(), "CONFLICT") {
		t.Fatalf("fixed release conflict not surfaced: %v", err)
	}

	server5, _ := newReleaseStoreFixture(http.StatusServiceUnavailable, "")
	client5 := &PlatformClient{BaseURL: server5.URL, Token: "fixture-api-token", Client: server5.Client()}
	if _, _, err := client5.RegisterReleaseMaterial(context.Background(), validReleaseEntry()); err == nil || !errors.Is(err, ErrPlatformUnavailable) {
		t.Fatalf("availability not mapped: %v", err)
	}
}

func TestRegisterReleaseMaterialValidatesBeforeRequest(t *testing.T) {
	server, _ := newReleaseStoreFixture(http.StatusCreated, `{}`)
	client := &PlatformClient{BaseURL: server.URL, Token: "fixture-api-token", Client: server.Client()}
	for _, mutate := range []func(*ReleaseMaterialEntry){
		func(e *ReleaseMaterialEntry) { e.ReleaseID = "" },
		func(e *ReleaseMaterialEntry) { e.Role = "gateway" },
		func(e *ReleaseMaterialEntry) { e.LockDigest = "abcdef" },
		func(e *ReleaseMaterialEntry) { e.ContentDigest = "sha256:0000" },
	} {
		bad := validReleaseEntry()
		mutate(&bad)
		if _, _, err := client.RegisterReleaseMaterial(context.Background(), bad); err == nil || !strings.Contains(err.Error(), ErrRequestRejected.Error()) {
			t.Fatalf("invalid entry released: %v", err)
		}
	}
}

func TestListReleaseMaterialsDecodesDirectory(t *testing.T) {
	body := `{"releases":[` + releaseStoreOKBody() + `],"total":1}`
	server, _ := newReleaseStoreFixture(http.StatusOK, body)
	client := &PlatformClient{BaseURL: server.URL, Token: "fixture-api-token", Client: server.Client()}
	entries, err := client.ListReleaseMaterials(context.Background())
	if err != nil || len(entries) != 1 || entries[0].Role != string(RoleWorker) {
		t.Fatalf("list decoding failed: %+v %v", entries, err)
	}
}
