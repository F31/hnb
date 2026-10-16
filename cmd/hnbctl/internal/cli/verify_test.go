package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/F31/hnb/cmd/hnbctl/internal/toolkit"
)

// The CLI verify surface must show the full required catalog and never claim
// completion while every check is still unevaluated.
func TestVerifyCLIReportsRequiredCatalogAsUnverified(t *testing.T) {
	code, out, errOut := runCLI(t, "verify", "--format", "json")
	if code == ExitSuccess {
		t.Fatalf("verify reported success without any observation: %s %s", out, errOut)
	}
	if code != ExitNotVerified {
		t.Fatalf("expected not-verified exit, got %d (%s %s)", code, out, errOut)
	}
	var report toolkit.VerifyReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid verify JSON: %v", err)
	}
	if report.Delivery.CompleteDelivery || report.Delivery.Overall == toolkit.DeliverySuccess {
		t.Fatalf("unverified checks reported as complete: %+v", report.Delivery)
	}
	// Every required check id must be present and none may be verified.
	if len(report.MissingRequiredChecks()) != 0 {
		t.Fatalf("catalog incomplete: %v", report.MissingRequiredChecks())
	}
	required := 0
	for _, layer := range report.Layers {
		for _, check := range layer.Checks {
			required++
			if check.State == toolkit.LayerPassed {
				t.Fatalf("unevaluated check reported as passed: %+v", check)
			}
		}
	}
	if required < 13 {
		t.Fatalf("expected the full required check set, got %d checks", required)
	}
	// All four layers must be represented.
	if len(report.Layers) != len(toolkit.DeliveryLayers) {
		t.Fatalf("expected four layers, got %d", len(report.Layers))
	}
}

func TestVerifyRejectsBadFlags(t *testing.T) {
	code, _, errOut := runCLI(t, "verify", "--level", "galaxy")
	if code == ExitSuccess || !strings.Contains(errOut, "--level") {
		t.Fatalf("invalid level accepted: %d %s", code, errOut)
	}
	code, _, errOut = runCLI(t, "verify", "--format", "yaml")
	if code == ExitSuccess || !strings.Contains(errOut, "--format") {
		t.Fatalf("invalid format accepted: %d %s", code, errOut)
	}
	code, _, errOut = runCLI(t, "verify", "extra")
	if code == ExitSuccess || !strings.Contains(errOut, "unexpected") {
		t.Fatalf("positional argument accepted: %d %s", code, errOut)
	}
}

func TestVerifyLevelFiltersChecksAndCountsUnverified(t *testing.T) {
	for _, layer := range toolkit.DeliveryLayers {
		t.Run(string(layer), func(t *testing.T) {
			code, out, errOut := runCLI(t, "verify", "--level", string(layer), "--format", "json")
			if code != ExitNotVerified {
				t.Fatalf("scoped verify incorrectly passed: %d %s", code, errOut)
			}
			var report toolkit.VerifyReport
			if err := json.Unmarshal([]byte(out), &report); err != nil {
				t.Fatal(err)
			}
			if len(report.Layers) != 1 || report.Layers[0].Layer != layer {
				t.Fatalf("level did not filter output: %+v", report.Layers)
			}
			expected := len(toolkit.VerificationCatalog()[layer])
			if len(report.Layers[0].Checks) != expected || !strings.Contains(errOut, fmt.Sprintf("%d required checks unverified", expected)) {
				t.Fatalf("incorrect scoped check count: %s %s", out, errOut)
			}
			if report.Delivery.CompleteDelivery || report.Delivery.Overall == toolkit.DeliverySuccess {
				t.Fatal("scoped checks promoted to full delivery")
			}
			_, human, _ := runCLI(t, "verify", "--level", string(layer))
			for _, other := range toolkit.DeliveryLayers {
				if other != layer && strings.Contains(human, string(other)+" [") {
					t.Fatalf("human output includes another layer: %s", human)
				}
			}
		})
	}
}

func TestVerifyHumanOutputListsLayersWithoutSecrets(t *testing.T) {
	_, out, _ := runCLI(t, "verify")
	for _, want := range []string{"overall=", "node [", "cluster [", "platform [", "managed ["} {
		if !strings.Contains(out, want) {
			t.Fatalf("human verify output missing %q: %s", want, out)
		}
	}
	for _, forbidden := range []string{"password", "BEGIN EC PRIVATE KEY", "dsn="} {
		if strings.Contains(strings.ToLower(out), strings.ToLower(forbidden)) {
			t.Fatalf("verify output leaked %q: %s", forbidden, out)
		}
	}
}
