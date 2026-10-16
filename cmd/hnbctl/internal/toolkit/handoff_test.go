package toolkit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// handoffFixture emulates the platform: registration intent + agent observation.
func handoffFixture(t *testing.T, operationID string, agentConnected bool, observedAt time.Time) (*httptest.Server, *[]string) {
	t.Helper()
	var paths []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		switch {
		case strings.HasSuffix(r.URL.Path, "/runtime-intents"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(intentResponse{APIVersion: "hnb.io/v1", IntentID: "intent-1", Status: "accepted", OperationID: operationID})
		case strings.HasPrefix(r.URL.Path, "/api/v1/resources/clusters/"):
			w.Header().Set("Content-Type", "application/json")
			observed := ""
			if !observedAt.IsZero() {
				observed = observedAt.UTC().Format(time.RFC3339)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": strings.TrimPrefix(r.URL.Path, "/api/v1/resources/clusters/"), "agentConnected": agentConnected, "observedAt": observed, "lastKnownState": "Running"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	return server, &paths
}

func sampleRun(t *testing.T) *InstallRun {
	t.Helper()
	store := fixedStore(t)
	if _, err := store.Create("run-handoff", fixtureConfigDigest, fixtureLockDigest, "existing", []string{"namespace/hnb-system"}); err != nil {
		t.Fatal(err)
	}
	run, err := store.AcquireWriter("writer-a", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	for _, phase := range []PhaseName{PhasePreparing, PhaseDatabaseMigrate, PhasePlatformRelease, PhasePlatformVerified} {
		if _, err := store.BeginPhase("writer-a", run.Epoch, phase, always(true, "")); err != nil {
			t.Fatal(err)
		}
		if _, err := store.CompletePhase("writer-a", run.Epoch, phase, always(true, ""), nil); err != nil {
			t.Fatal(err)
		}
	}
	state, err := store.read()
	if err != nil {
		t.Fatal(err)
	}
	return state
}

// A real registration Operation plus a live agent observation completes handoff.
func TestHandoffCompletesOnlyWithOperationAndLiveObservation(t *testing.T) {
	now := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	server, paths := handoffFixture(t, "op-register-1", true, now.Add(5*time.Minute))
	client := &PlatformClient{BaseURL: server.URL, Token: "fixture-api-token", Client: server.Client()}
	result, err := client.Handoff(context.Background(), "target-1", "idem-handoff", "corr-1", sampleRun(t), now)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || result.RegistrationOperationID != "op-register-1" {
		t.Fatalf("handoff did not complete with operation + observation: %+v", result)
	}
	if result.Observation == nil || !result.Observation.Authenticated {
		t.Fatalf("missing authenticated observation: %+v", result.Observation)
	}
	// Both the formal registration and the observation query really happened.
	joined := strings.Join(*paths, " | ")
	if !strings.Contains(joined, "POST /api/v1/runtime-intents") || !strings.Contains(joined, "GET /api/v1/resources/clusters/target-1") {
		t.Fatalf("handoff did not use the formal registration and observation contracts: %s", joined)
	}
}

// The receipt alone must never complete a handoff: no registration Operation.
func TestReceiptAloneNeverCompletesHandoff(t *testing.T) {
	now := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	server, _ := handoffFixture(t, "", true, now.Add(5*time.Minute))
	client := &PlatformClient{BaseURL: server.URL, Token: "fixture-api-token", Client: server.Client()}
	result, err := client.Handoff(context.Background(), "target-1", "idem", "corr-1", sampleRun(t), now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Complete {
		t.Fatalf("handoff completed without a registration Operation: %+v", result)
	}
	if len(result.Reasons) == 0 {
		t.Fatal("missing handoff reason")
	}
}

// 初始化阶段时间不能被伪装成 Agent 实时观测时间.
func TestInstallerPhaseTimesCannotMasqueradeAsObservation(t *testing.T) {
	now := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	// The server reports an observation that predates the handoff.
	server, _ := handoffFixture(t, "op-1", true, now.Add(-time.Hour))
	client := &PlatformClient{BaseURL: server.URL, Token: "fixture-api-token", Client: server.Client()}
	result, err := client.Handoff(context.Background(), "target-1", "idem", "corr-1", sampleRun(t), now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Complete {
		t.Fatal("a stale observation completed the handoff")
	}
	if !strings.Contains(strings.Join(result.Reasons, " "), "installer times cannot stand in") {
		t.Fatalf("missing stale-observation reason: %+v", result.Reasons)
	}
}

// A disconnected or unauthenticated agent does not complete handoff.
func TestUnauthenticatedAgentDoesNotCompleteHandoff(t *testing.T) {
	now := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	server, _ := handoffFixture(t, "op-1", false, now.Add(time.Minute))
	client := &PlatformClient{BaseURL: server.URL, Token: "fixture-api-token", Client: server.Client()}
	result, err := client.Handoff(context.Background(), "target-1", "idem", "corr-1", sampleRun(t), now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Complete || !strings.Contains(strings.Join(result.Reasons, " "), "not authenticated") {
		t.Fatalf("unauthenticated agent completed handoff: %+v", result)
	}
}

// Missing observation time is not a live observation.
func TestMissingObservationTimeDoesNotComplete(t *testing.T) {
	now := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	server, _ := handoffFixture(t, "op-1", true, time.Time{})
	client := &PlatformClient{BaseURL: server.URL, Token: "fixture-api-token", Client: server.Client()}
	result, err := client.Handoff(context.Background(), "target-1", "idem", "corr-1", sampleRun(t), now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Complete {
		t.Fatal("handoff completed with no observation time")
	}
}

// Platform unavailability is reported, never turned into a local handoff.
func TestHandoffUnavailableDoesNotFabricateObservation(t *testing.T) {
	now := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	client := &PlatformClient{BaseURL: server.URL, Token: "fixture-api-token", Client: server.Client()}
	result, err := client.Handoff(context.Background(), "target-1", "idem", "corr-1", sampleRun(t), now)
	if err == nil {
		t.Fatal("unavailable platform reported a successful handoff")
	}
	if result.Observation != nil || result.Complete {
		t.Fatalf("unavailable platform produced a fabricated observation: %+v", result)
	}
	if len(result.Reasons) == 0 {
		t.Fatal("unavailable handoff must record a reason")
	}
}

// The attachment carries installer phase times and never claims to be an
// operation or a read-model result.
func TestHandoffAttachmentCarriesOnlyLocalFacts(t *testing.T) {
	run := sampleRun(t)
	now := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	attachment, err := NewHandoffAttachment(run, now)
	if err != nil {
		t.Fatal(err)
	}
	if attachment.SchemaVersion != HandoffSchema || attachment.RunID != run.RunID {
		t.Fatalf("attachment identity wrong: %+v", attachment)
	}
	if len(attachment.PhaseTimes) == 0 {
		t.Fatal("attachment must carry installer phase times for correlation")
	}
	data, _ := json.Marshal(attachment)
	for _, forbidden := range []string{"operationId", "status\": \"succeeded", "readModel", "password", "kubeconfig"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("attachment leaked a fabricated fact %q: %s", forbidden, data)
		}
	}
	if _, err := NewHandoffAttachment(nil, now); err == nil {
		t.Fatal("handoff attachment accepted without a run")
	}
}

// Invalid target ids are refused before any request.
func TestHandoffRejectsInvalidTargetID(t *testing.T) {
	var called bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { called = true }))
	defer server.Close()
	client := &PlatformClient{BaseURL: server.URL, Token: "t", Client: server.Client()}
	if _, err := client.GetAgentObservation(context.Background(), "bad id"); err == nil {
		t.Fatal("invalid target id accepted")
	}
	if called {
		t.Fatal("invalid target id produced a request")
	}
}
