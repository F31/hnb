package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"
)

// Handoff never reports success without both a registration Operation and a
// live agent observation, and the local receipt never substitutes for either.
func TestHandoffRequiresOperationAndObservation(t *testing.T) {
	server := platformFixture(t, 200) // intent fixture; cluster read returns 404 -> no observation
	token := writeTokenFile(t)
	dir := t.TempDir()
	runDir := writeRun(t, dir, toolkit.PhaseSucceededState, map[string]string{
		string(toolkit.PhasePlatformVerified): "succeeded",
	})
	code, out, errOut := runCLI(t, "handoff", "--run-dir", runDir, "--target-id", "target-1",
		"--idempotency-key", "idem-1", "--platform-url", server.URL, "--token-file", token, "--format", "json")
	if code == ExitSuccess {
		t.Fatalf("handoff reported success without a live agent observation: %s %s", out, errOut)
	}
	var result struct {
		Complete  bool     `json:"complete"`
		Reasons   []string `json:"reasons"`
		Operation string   `json:"registrationOperationId"`
	}
	if err := json.Unmarshal([]byte(out), &result); err == nil && result.Complete {
		t.Fatalf("handoff marked complete without observation: %s", out)
	}
}

func TestHandoffRequiresMandatoryFlags(t *testing.T) {
	token := writeTokenFile(t)
	for _, args := range [][]string{
		{"handoff", "--target-id", "t1", "--idempotency-key", "k"},
		{"handoff", "--run-dir", t.TempDir(), "--idempotency-key", "k"},
		{"handoff", "--run-dir", t.TempDir(), "--target-id", "t1"},
	} {
		code, _, _ := runCLI(t, append(args, "--platform-url", "https://api.invalid", "--token-file", token)...)
		if code == ExitSuccess {
			t.Fatalf("handoff accepted incomplete flags: %v", args)
		}
	}
}

// A platform that is unavailable reports a connection failure; handoff never
// invents a registration Operation or agent observation locally.
func TestHandoffUnavailableNeverFabricatesObservation(t *testing.T) {
	server := platformFixture(t, 503)
	token := writeTokenFile(t)
	dir := t.TempDir()
	runDir := writeRun(t, dir, toolkit.PhaseSucceededState, map[string]string{
		string(toolkit.PhasePlatformVerified): "succeeded",
	})
	code, out, errOut := runCLI(t, "handoff", "--run-dir", runDir, "--target-id", "target-1",
		"--idempotency-key", "idem-1", "--platform-url", server.URL, "--token-file", token, "--format", "json")
	if code != ExitConnectionFailed {
		t.Fatalf("unavailable platform must map to connection failure, got %d (%s %s)", code, out, errOut)
	}
	var result struct {
		Complete    bool   `json:"complete"`
		Observation any    `json:"observation"`
		Operation   string `json:"registrationOperationId"`
	}
	if err := json.Unmarshal([]byte(out), &result); err == nil {
		if result.Complete || result.Observation != nil || result.Operation != "" {
			t.Fatalf("unavailable platform produced fabricated handoff facts: %s", out)
		}
	}
	combined := out + errOut
	for _, forbidden := range []string{"agent-token", "BEGIN EC PRIVATE KEY", "password"} {
		if strings.Contains(combined, forbidden) {
			t.Fatalf("handoff output leaked %q: %s", forbidden, combined)
		}
	}
}
