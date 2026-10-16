package toolkit

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// VerifyCheck is one independent assertion inside a delivery layer. Checks are
// never aggregated into a single "healthy" signal: a passing check does not
// imply any other check in the same or a higher layer passed.
type VerifyCheck struct {
	CheckID     string        `json:"checkId"`
	Layer       DeliveryLayer `json:"layer"`
	Expected    string        `json:"expected"`
	Actual      string        `json:"actual"`
	State       string        `json:"state"`
	ReasonCode  string        `json:"reasonCode,omitempty"`
	Remediation string        `json:"remediation,omitempty"`
	ObservedAt  time.Time     `json:"observedAt"`
	// OwnedTestResource marks a check that created a temporary resource of its
	// own. Such a resource must be cleaned up; cleanup is tracked separately so
	// a leftover can never be reported as a successful verification.
	OwnedTestResource bool `json:"ownedTestResource,omitempty"`
}

// VerifyLayer groups checks by delivery layer.
type VerifyLayer struct {
	Layer  DeliveryLayer `json:"layer"`
	State  string        `json:"state"`
	Checks []VerifyCheck `json:"checks"`
}

// CleanupResult records disposal of verification-owned test resources.
type CleanupResult struct {
	Resource string `json:"resource"`
	Removed  bool   `json:"removed"`
	Reason   string `json:"reason,omitempty"`
}

// VerifyReport is the layered acceptance result. It reuses the delivery
// verdict rules: overall success requires every layer independently verified.
type VerifyReport struct {
	SchemaVersion string          `json:"schemaVersion"`
	Layers        []VerifyLayer   `json:"layers"`
	Delivery      *DeliveryReport `json:"delivery"`
	Cleanup       []CleanupResult `json:"cleanup,omitempty"`
	GeneratedAt   time.Time       `json:"generatedAt"`
	// LeftoverOwnedResources blocks acceptance when the tool's own test
	// resources were not removed.
	LeftoverOwnedResources []string `json:"leftoverOwnedResources,omitempty"`
}

// VerifySpec declares a check and how to evaluate it.
type VerifySpec struct {
	CheckID     string
	Layer       DeliveryLayer
	Expected    string
	Probe       func(ctx context.Context) (actual string, passed bool, err error)
	Remediation string
	// Cleanup removes a resource this check itself created. It is only ever
	// called for checks that declared OwnedTestResource.
	Cleanup func(ctx context.Context) error
	// OwnedTestResource marks self-created test resources.
	OwnedTestResource bool
}

// RunVerify executes every check, records its independent verdict and cleans up
// the resources the checks themselves created. Cleanup failures are reported
// and never converted into a passing verification.
func RunVerify(ctx context.Context, specs []VerifySpec, now time.Time) *VerifyReport {
	report := &VerifyReport{SchemaVersion: "verify-report/v1", GeneratedAt: now}
	if now.IsZero() {
		report.GeneratedAt = time.Now().UTC()
		now = report.GeneratedAt
	}
	byLayer := map[DeliveryLayer][]VerifyCheck{}
	for _, spec := range specs {
		check := VerifyCheck{CheckID: spec.CheckID, Layer: spec.Layer, Expected: spec.Expected,
			Remediation: spec.Remediation, ObservedAt: now, OwnedTestResource: spec.OwnedTestResource}
		if spec.Probe == nil {
			check.State, check.ReasonCode = LayerBlocked, "VERIFY_CHECK_MISSING"
			check.Actual = "no probe supplied"
		} else {
			actual, passed, err := spec.Probe(ctx)
			check.Actual = actual
			switch {
			case err != nil:
				check.State, check.ReasonCode = LayerBlocked, "VERIFY_PROBE_UNAVAILABLE"
			case passed:
				check.State, check.ReasonCode = LayerPassed, "CHECK_PASSED"
			default:
				check.State, check.ReasonCode = LayerFailed, "VERIFY_CHECK_FAILED"
			}
		}
		byLayer[spec.Layer] = append(byLayer[spec.Layer], check)

		// Clean up only resources this verification created itself.
		if spec.OwnedTestResource && spec.Cleanup != nil {
			entry := CleanupResult{Resource: spec.CheckID}
			if cleanupErr := spec.Cleanup(ctx); cleanupErr != nil {
				entry.Removed, entry.Reason = false, "cleanup failed"
				report.Cleanup = append(report.Cleanup, entry)
				report.LeftoverOwnedResources = append(report.LeftoverOwnedResources, spec.CheckID)
			} else {
				entry.Removed = true
				report.Cleanup = append(report.Cleanup, entry)
			}
		}
	}

	outcomes := map[DeliveryLayer]LayerOutcome{}
	for _, layer := range DeliveryLayers {
		checks := byLayer[layer]
		verdict := LayerOutcome{Layer: layer, State: LayerPassed, ReasonCode: "CHECK_PASSED", ObservedAt: now}
		if len(checks) == 0 {
			verdict.State, verdict.ReasonCode = LayerSkipped, "LAYER_NOT_VERIFIED"
		}
		for _, check := range checks {
			switch check.State {
			case LayerPassed:
			case LayerBlocked:
				if verdict.State == LayerPassed || verdict.State == LayerFailed {
					verdict = LayerOutcome{Layer: layer, State: LayerBlocked, ReasonCode: check.ReasonCode, ObservedAt: now, Detail: check.CheckID}
				}
			default:
				verdict = LayerOutcome{Layer: layer, State: LayerFailed, ReasonCode: check.ReasonCode, ObservedAt: now, Detail: check.CheckID}
			}
		}
		report.Layers = append(report.Layers, VerifyLayer{Layer: layer, State: verdict.State, Checks: checks})
		outcomes[layer] = verdict
	}
	sort.Slice(report.Cleanup, func(i, j int) bool { return report.Cleanup[i].Resource < report.Cleanup[j].Resource })
	sort.Strings(report.LeftoverOwnedResources)

	report.Delivery = NewDeliveryReport("", outcomes)
	// Required checks that were never evaluated block acceptance exactly like a
	// failure: a subset of healthy layers must never be promoted to full
	// delivery success.
	if missing := report.MissingRequiredChecks(); len(missing) > 0 {
		report.Delivery.CompleteDelivery = false
		if report.Delivery.Overall == DeliverySuccess {
			report.Delivery.Overall = DeliveryPartial
		}
		report.Delivery.FailedLayers = append(report.Delivery.FailedLayers, LayerPlatform)
	}
	// A leftover resource owned by this tool blocks acceptance even when every
	// check passed: the verification is not clean.
	if len(report.LeftoverOwnedResources) > 0 {
		report.Delivery.CompleteDelivery = false
		if report.Delivery.Overall == DeliverySuccess {
			report.Delivery.Overall = DeliveryPartial
		}
		if len(report.Delivery.FailedLayers) == 0 {
			report.Delivery.FailedLayers = []DeliveryLayer{LayerPlatform}
		}
	}
	return report
}

// MissingLayers lists layers that were not verified at all, so acceptance never
// silently skips a required layer.
func (r *VerifyReport) MissingLayers() []string {
	var missing []string
	for _, layer := range r.Layers {
		if layer.State == LayerSkipped {
			missing = append(missing, string(layer.Layer))
		}
	}
	return missing
}

// FormatVerifyHuman renders the layered result without credential material.
func FormatVerifyHuman(w interface{ Write([]byte) (int, error) }, r *VerifyReport) {
	fmt.Fprintf(w, "overall=%s completeDelivery=%v\n", r.Delivery.Overall, r.Delivery.CompleteDelivery)
	for _, layer := range r.Layers {
		fmt.Fprintf(w, "  %s [%s]\n", layer.Layer, layer.State)
		for _, check := range layer.Checks {
			fmt.Fprintf(w, "    %-28s %-8s %s %s\n", check.CheckID, check.State, check.ReasonCode, SafeReportText(check.Actual))
		}
	}
	for _, cleanup := range r.Cleanup {
		fmt.Fprintf(w, "  cleanup %-28s removed=%v %s\n", cleanup.Resource, cleanup.Removed, cleanup.Reason)
	}
	if missing := r.MissingLayers(); len(missing) > 0 {
		fmt.Fprintf(w, "  unverified layers: %s\n", strings.Join(missing, ","))
	}
	if len(r.LeftoverOwnedResources) > 0 {
		fmt.Fprintf(w, "  leftover owned test resources: %s\n", strings.Join(r.LeftoverOwnedResources, ","))
	}
}

// ProbeFunc is the signature every externally observed fact must satisfy. It is
// supplied by the caller (which owns the transport); this package never dials a
// cluster or platform on its own during verification planning.
type ProbeFunc func(ctx context.Context) (actual string, passed bool, err error)

// VerificationCatalog declares the required checks per layer. A missing entry
// leaves its layer unverified rather than implicitly passed.
func VerificationCatalog() map[DeliveryLayer][]string {
	return map[DeliveryLayer][]string{
		LayerNode:     {"node.readiness", "node.owner-conflict"},
		LayerCluster:  {"cluster.api", "cluster.control-plane-ready", "cluster.cni-ready"},
		LayerPlatform: {"platform.entry-tls", "platform.login", "platform.pg", "platform.jetstream", "platform.provider-route", "platform.app-operation", "platform.agent-observation", "platform.read-model"},
		LayerManaged:  {"managed.registration-operation", "managed.agent-authenticated"},
	}
}

// RequiredCheckIDs returns every check id required for complete acceptance.
func RequiredCheckIDs() map[DeliveryLayer]map[string]bool {
	out := map[DeliveryLayer]map[string]bool{}
	for layer, ids := range VerificationCatalog() {
		set := map[string]bool{}
		for _, id := range ids {
			set[id] = true
		}
		out[layer] = set
	}
	return out
}

// MissingRequiredChecks reports required checks that the run did not actually
// evaluate, so a healthy-looking subset cannot be reported as full delivery.
func (r *VerifyReport) MissingRequiredChecks() []string {
	required := RequiredCheckIDs()
	seen := map[string]bool{}
	for _, layer := range r.Layers {
		for _, check := range layer.Checks {
			seen[check.CheckID] = true
		}
	}
	var missing []string
	for _, layer := range DeliveryLayers {
		for id := range required[layer] {
			if !seen[id] {
				missing = append(missing, string(layer)+"/"+id)
			}
		}
	}
	sort.Strings(missing)
	return missing
}

// PendingVerificationSpecs returns the full required catalog with every probe
// deliberately unevaluated. It keeps required checks visible while guaranteeing
// that none of them can be reported as verified.
func PendingVerificationSpecs() []VerifySpec {
	var specs []VerifySpec
	for _, layer := range DeliveryLayers {
		for _, id := range VerificationCatalog()[layer] {
			specs = append(specs, VerifySpec{CheckID: id, Layer: layer,
				Expected: "independent observation required",
				Probe: func(context.Context) (string, bool, error) {
					return "no bound deployment observation source", false, nil
				}})
		}
	}
	return specs
}
