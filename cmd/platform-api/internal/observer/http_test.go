package observer

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeVerifier struct {
	identity Identity
	err      error
}

func (f *fakeVerifier) VerifyObserverIdentity(_ context.Context, _ string) (Identity, error) {
	return f.identity, f.err
}

type fakeProjectorStore struct {
	accepted   []*Observation
	lastDigest string
}

func (f *fakeProjectorStore) SaveObservation(_ context.Context, o *Observation, _ Identity, digest string) error {
	f.accepted = append(f.accepted, o)
	f.lastDigest = digest
	return nil
}

func (f *fakeProjectorStore) LoadCursor(context.Context, string, string, string) (cursor, bool, error) {
	return cursor{}, false, nil
}

func (f *fakeProjectorStore) ApplySourceReset(context.Context, *SourceReset, Identity) error {
	return nil
}
func (f *fakeProjectorStore) RecordReplay(context.Context, *Observation) error     { return nil }
func (f *fakeProjectorStore) RecordGap(context.Context, *Observation, int64) error { return nil }

func TestIngestHandlerAcceptsValidObservation(t *testing.T) {
	st := &fakeProjectorStore{}
	h := NewIngestHandler(NewProjector(st), &fakeVerifier{identity: testIdentity()})
	_, obsHandler, _, _ := h.Routes()
	server := httptest.NewServer(http.HandlerFunc(obsHandler))
	defer server.Close()

	req, _ := http.NewRequest(http.MethodPost, server.URL, strings.NewReader(string(baseObservation(1))))
	req.Header.Set("Authorization", "Bearer token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if len(st.accepted) != 1 {
		t.Fatalf("accepted=%d want 1", len(st.accepted))
	}
	if st.accepted[0].Sequence != 1 {
		t.Fatalf("sequence=%d", st.accepted[0].Sequence)
	}
}

func TestIngestHandlerRejectsUnauthenticated(t *testing.T) {
	h := NewIngestHandler(NewProjector(&fakeProjectorStore{}), &fakeVerifier{err: errors.New("denied")})
	_, obsHandler, _, _ := h.Routes()
	server := httptest.NewServer(http.HandlerFunc(obsHandler))
	defer server.Close()

	req, _ := http.NewRequest(http.MethodPost, server.URL, strings.NewReader(string(baseObservation(1))))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestIngestHandlerRejectsOversizedPayload(t *testing.T) {
	h := NewIngestHandler(NewProjector(&fakeProjectorStore{}), &fakeVerifier{identity: testIdentity()})
	_, obsHandler, _, _ := h.Routes()
	server := httptest.NewServer(http.HandlerFunc(obsHandler))
	defer server.Close()

	big := strings.Repeat("x", MaxObservationPayload+1)
	req, _ := http.NewRequest(http.MethodPost, server.URL, strings.NewReader(big))
	req.Header.Set("Authorization", "Bearer token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestIngestHandlerReset(t *testing.T) {
	h := NewIngestHandler(NewProjector(&fakeProjectorStore{}), &fakeVerifier{identity: testIdentity()})
	_, _, _, resetHandler := h.Routes()
	server := httptest.NewServer(http.HandlerFunc(resetHandler))
	defer server.Close()

	reset := map[string]any{
		"schemaVersion": "1.0.0", "eventId": "6d384d43-243b-5e14-b7e4-c03be376cb7c",
		"tenantId": "tenant-a", "targetId": "515eba09-0a41-5b92-b972-69af1f0f655c",
		"targetKind": "KubernetesTarget", "observerId": "agent-1", "observerKind": "Agent",
		"previousObserverGeneration": 1, "newObserverGeneration": 2,
		"observedAt": time.Now().UTC().Add(-time.Second), "observerLeaseId": "6d384d43-243b-5e14-b7e4-c03be376cb7c",
		"reason": "observer-restarted",
	}
	body, _ := json.Marshal(reset)
	req, _ := http.NewRequest(http.MethodPost, server.URL, strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bearer token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}
