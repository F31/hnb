package lifecycle

import (
	"errors"
	"testing"
	"time"
)

const digest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestValidateCommandCarriesMetadataOnly(t *testing.T) {
	cmd := Command{ProviderID: "p1", ProviderVersion: "1.0.0", Action: ActionInstall, BundleDigest: digest, OperationID: "op1", IdempotencyKey: "idem1", CapabilityIDs: []string{"c1"}, SecretReferences: []string{"secret-ref/provider/p1"}}
	if err := ValidateCommand(cmd); err != nil {
		t.Fatal(err)
	}
	cmd.SecretReferences = []string{"token=plain"}
	if err := ValidateCommand(cmd); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("expected inline secret rejection, got %v", err)
	}
}

func TestValidateEventRejectsSecretValues(t *testing.T) {
	if err := ValidateEventFields(map[string]any{"provider_id": "p1", "secret_references": []string{"ref"}}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateEventFields(map[string]any{"access_token": "plain"}); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("expected secret-like field rejection, got %v", err)
	}
}

func TestBackoffIsBounded(t *testing.T) {
	if Backoff(10) != 30*time.Second {
		t.Fatalf("backoff not bounded: %s", Backoff(10))
	}
}

func TestPromoteCandidateKeepsActiveOnFailedHealth(t *testing.T) {
	active := Snapshot{ProviderID: "p1", ProviderVersion: "1", Active: true}
	candidate := Snapshot{ProviderID: "p1", ProviderVersion: "2"}
	next, _, err := PromoteCandidate(active, candidate, false)
	if err == nil || !next.Active || next.ProviderVersion != "1" {
		t.Fatalf("active provider not preserved: next=%+v err=%v", next, err)
	}
}

func TestUninstallBlockedReportsDependencies(t *testing.T) {
	err := EnsureUninstallAllowed(DependencyReport{ActiveOperations: 1, NavigationRoutes: 1})
	if !errors.Is(err, ErrUninstallBlocked) {
		t.Fatalf("expected block, got %v", err)
	}
}
