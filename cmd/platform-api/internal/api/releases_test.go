package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/F31/hnb/cmd/platform-api/internal/store"
	"github.com/google/uuid"
)

func (f *fakeStore) RegisterReleaseMaterial(_ context.Context, entry store.ReleaseCatalogEntry) (*store.ReleaseCatalogEntry, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.releases == nil {
		f.releases = map[string]store.ReleaseCatalogEntry{}
	}
	key := entry.ReleaseID + "/" + entry.ReleaseVersion + "/" + entry.Role
	if existing, ok := f.releases[key]; ok {
		if existing.ContentDigest != entry.ContentDigest || existing.LockDigest != entry.LockDigest {
			return nil, false, store.ErrFixedReleaseConflict
		}
		return &existing, false, nil
	}
	entry.ID = uuid.NewString()
	entry.RegisteredAt = time.Now().UTC()
	f.releases[key] = entry
	return &entry, true, nil
}

func (f *fakeStore) ListReleaseMaterials(_ context.Context) ([]store.ReleaseCatalogEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []store.ReleaseCatalogEntry
	for _, e := range f.releases {
		out = append(out, e)
	}
	return out, nil
}

func releaseDigest(hex rune) string {
	return "sha256:" + strings.Repeat(string(hex), 64)
}

func releaseMaterialBody(t *testing.T, override map[string]string) string {
	t.Helper()
	body := map[string]string{
		"releaseId": "r1", "releaseVersion": "1.0", "role": "worker",
		"bundleId": "bundle-1111111111111111", "lockDigest": releaseDigest('1'),
		"contentDigest": releaseDigest('2'),
	}
	for k, v := range override {
		body[k] = v
	}
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func doReleaseRequest(t *testing.T, srv *Server, method, token, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, "/v1/releases", strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w, out
}

func TestReleaseMaterialRegistration(t *testing.T) {
	for _, test := range []struct {
		name    string
		permit  bool
		body    string
		status  int
		code    string
		prepare func(*fakeStore)
	}{
		{name: "created", permit: true, body: releaseMaterialBody(t, nil), status: http.StatusCreated},
		{name: "permission denied", body: releaseMaterialBody(t, nil), status: http.StatusForbidden, code: "FORBIDDEN"},
		{name: "bad role", permit: true, body: releaseMaterialBody(t, map[string]string{"role": "gateway"}), status: http.StatusBadRequest},
		{name: "bad lock digest", permit: true, body: releaseMaterialBody(t, map[string]string{"lockDigest": "abcdef"}), status: http.StatusBadRequest},
		{name: "bad content digest", permit: true, body: releaseMaterialBody(t, map[string]string{"contentDigest": "sha256:0000"}), status: http.StatusBadRequest},
		{name: "empty release id", permit: true, body: releaseMaterialBody(t, map[string]string{"releaseId": ""}), status: http.StatusBadRequest},
		{name: "unknown field", permit: true, body: `{"releaseId":"r1","releaseVersion":"1.0","role":"worker","bundleId":"b","lockDigest":"` + releaseDigest('1') + `","contentDigest":"` + releaseDigest('2') + `","surprise":true}`, status: http.StatusBadRequest},
		{name: "fixed release replaced", permit: true, body: releaseMaterialBody(t, nil), status: http.StatusConflict, code: "CONFLICT", prepare: func(st *fakeStore) {
			st.releases["r1/1.0/worker"] = store.ReleaseCatalogEntry{ReleaseID: "r1", ReleaseVersion: "1.0", Role: "worker", LockDigest: releaseDigest('1'), ContentDigest: releaseDigest('9')}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			st := newFakeStore()
			if test.prepare != nil {
				test.prepare(st)
			}
			var srv *Server
			if test.permit {
				srv = NewServer(st, testAuthenticator{})
			} else {
				srv = NewServer(st, noPermissionAuthenticator{})
			}
			w, resp := doReleaseRequest(t, srv, http.MethodPost, "valid-token", test.body)
			if w.Code != test.status {
				t.Fatalf("status = %d (%v), want %d", w.Code, resp, test.status)
			}
			if test.code != "" && resp["code"] != test.code {
				t.Fatalf("code = %v, want %q", resp["code"], test.code)
			}
		})
	}
}

func TestReleaseMaterialRegistrationIdempotent(t *testing.T) {
	st := newFakeStore()
	srv := NewServer(st, testAuthenticator{})
	body := releaseMaterialBody(t, nil)
	first, _ := doReleaseRequest(t, srv, http.MethodPost, "valid-token", body)
	if first.Code != http.StatusCreated {
		t.Fatalf("first import status = %d", first.Code)
	}
	again, _ := doReleaseRequest(t, srv, http.MethodPost, "valid-token", body)
	if again.Code != http.StatusOK {
		t.Fatalf("duplicate import status = %d, want 200", again.Code)
	}
	entries, err := st.ListReleaseMaterials(context.Background())
	if err != nil || len(entries) != 1 {
		t.Fatalf("duplicate import changed catalog: %d %v", len(entries), err)
	}
}

func TestReleaseMaterialList(t *testing.T) {
	st := newFakeStore()
	srv := NewServer(st, testAuthenticator{})
	if w, _ := doReleaseRequest(t, srv, http.MethodPost, "valid-token", releaseMaterialBody(t, nil)); w.Code != http.StatusCreated {
		t.Fatalf("register status = %d", w.Code)
	}
	w, resp := doReleaseRequest(t, srv, http.MethodGet, "valid-token", "")
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d", w.Code)
	}
	if resp["total"] != float64(1) {
		t.Fatalf("list total = %v", resp["total"])
	}
	releases, ok := resp["releases"].([]any)
	if !ok || len(releases) != 1 {
		t.Fatalf("list releases = %v", resp["releases"])
	}
	first := releases[0].(map[string]any)
	if first["role"] != "worker" || first["releaseId"] != "r1" {
		t.Fatalf("list member = %v", first)
	}
	denied := NewServer(newFakeStore(), noPermissionAuthenticator{})
	if w, _ := doReleaseRequest(t, denied, http.MethodGet, "valid-token", ""); w.Code != http.StatusForbidden {
		t.Fatal("list did not enforce permission")
	}
}
