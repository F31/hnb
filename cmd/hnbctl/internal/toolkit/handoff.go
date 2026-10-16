package toolkit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HandoffSchema is the local handoff record. It is an *attachment* describing
// what this installer did; it is never a substitute for the platform Operation
// store and it never carries a fabricated Read Model result.
const HandoffSchema = "install-handoff/v1"

// HandoffAttachment records local initialization facts for later correlation.
type HandoffAttachment struct {
	SchemaVersion string `json:"schemaVersion"`
	RunID         string `json:"runId"`
	ConfigDigest  string `json:"configDigest"`
	LockDigest    string `json:"lockDigest"`
	// PhaseTimes are *installer* timestamps. They describe when this tool ran a
	// step locally and must never be presented as Agent observation times.
	PhaseTimes map[string]time.Time `json:"phaseTimes"`
	AttachedAt time.Time            `json:"attachedAt"`
}

// AgentObservation is server-authoritative evidence that a real Agent
// authenticated and reported the target. Only the platform can produce it.
type AgentObservation struct {
	TargetID      string    `json:"targetId"`
	Authenticated bool      `json:"authenticated"`
	ObservedAt    time.Time `json:"observedAt"`
	State         string    `json:"state"`
}

// HandoffResult is the outcome of establishing a first management relationship.
type HandoffResult struct {
	SchemaVersion string `json:"schemaVersion"`
	TargetID      string `json:"targetId"`
	// RegistrationOperationID is the formal server-side registration Operation.
	// It is empty when no such Operation was returned.
	RegistrationOperationID string            `json:"registrationOperationId,omitempty"`
	Attachment              HandoffAttachment `json:"attachment"`
	Observation             *AgentObservation `json:"observation,omitempty"`
	// Complete is true only when a real registration Operation AND a real Agent
	// observation both exist. A receipt alone never completes a handoff.
	Complete bool     `json:"complete"`
	Reasons  []string `json:"reasons,omitempty"`
}

// NewHandoffAttachment builds the local attachment from a run state. Phase
// timestamps are labelled as installer times.
func NewHandoffAttachment(run *InstallRun, attachedAt time.Time) (HandoffAttachment, error) {
	if run == nil {
		return HandoffAttachment{}, fmt.Errorf("install run is required for handoff attachment")
	}
	phases := map[string]time.Time{}
	for name, phase := range run.Phases {
		if phase == nil {
			continue
		}
		when := phase.FinishedAt
		if when.IsZero() {
			when = phase.StartedAt
		}
		if !when.IsZero() {
			phases[name] = when
		}
	}
	return HandoffAttachment{SchemaVersion: HandoffSchema, RunID: run.RunID,
		ConfigDigest: run.ConfigDigest, LockDigest: run.LockDigest,
		PhaseTimes: phases, AttachedAt: attachedAt}, nil
}

// Handoff establishes the initial management relationship for an existing
// install: a formal registration Intent is submitted through the platform, and
// the target is only considered handed off once the server reports a real Agent
// observation. The local receipt is attached as correlation evidence only.
func (p *PlatformClient) Handoff(ctx context.Context, targetID, idempotencyKey, correlationID string, run *InstallRun, now time.Time) (HandoffResult, error) {
	result := HandoffResult{SchemaVersion: HandoffSchema, TargetID: targetID}
	attachment, err := NewHandoffAttachment(run, now)
	if err != nil {
		return result, err
	}
	result.Attachment = attachment

	// 1. Formal registration through the platform (typed Intent, never a direct
	//    Read Model write).
	intent, err := p.SubmitIntent(ctx, IntentImportRuntimeTarget, idempotencyKey, correlationID,
		IntentSpec{TargetID: targetID, TargetKind: "kubernetes"})
	if err != nil {
		result.Reasons = append(result.Reasons, "registration intent not accepted: "+err.Error())
		return result, err
	}
	result.RegistrationOperationID = intent.OperationID
	if result.RegistrationOperationID == "" {
		result.Reasons = append(result.Reasons, "registration returned no Operation id")
	}

	// 2. Real Agent observation from the server. An install receipt is never a
	//    substitute for this.
	observation, err := p.GetAgentObservation(ctx, targetID)
	if err != nil {
		result.Reasons = append(result.Reasons, "agent observation unavailable: "+err.Error())
		return result, err
	}
	if !observation.Authenticated {
		result.Reasons = append(result.Reasons, "agent is not authenticated for this target")
	}
	// The observation must postdate the install handoff; an installer timestamp
	// can never stand in for a live observation.
	if !observation.ObservedAt.IsZero() && !observation.ObservedAt.After(now) {
		result.Reasons = append(result.Reasons, "agent observation is not newer than the handoff; installer times cannot stand in for live observation")
	}
	result.Observation = &observation
	result.Complete = result.RegistrationOperationID != "" && observation.Authenticated &&
		!observation.ObservedAt.IsZero() && observation.ObservedAt.After(now)
	if !result.Complete && len(result.Reasons) == 0 {
		result.Reasons = append(result.Reasons, "handoff is incomplete without a registration Operation and a live agent observation")
	}
	return result, nil
}

// GetAgentObservation reads server-authoritative agent state for a target.
func (p *PlatformClient) GetAgentObservation(ctx context.Context, targetID string) (AgentObservation, error) {
	var obs AgentObservation
	if strings.TrimSpace(p.Token) == "" {
		return obs, ErrNotAuthenticated
	}
	if !validName(targetID) {
		return obs, fmt.Errorf("%w: invalid target id", ErrRequestRejected)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(p.BaseURL, "/")+"/api/v1/resources/clusters/"+targetID, nil)
	if err != nil {
		return obs, fmt.Errorf("%w: invalid platform URL", ErrRequestRejected)
	}
	req.Header.Set("Authorization", "Bearer "+p.Token)
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return obs, fmt.Errorf("%w: platform API unreachable", ErrPlatformUnavailable)
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 500 || resp.StatusCode == http.StatusServiceUnavailable {
		return obs, fmt.Errorf("%w: platform returned HTTP %d", ErrPlatformUnavailable, resp.StatusCode)
	}
	if resp.StatusCode >= 300 {
		return obs, fmt.Errorf("%w: platform returned HTTP %d", ErrRequestRejected, resp.StatusCode)
	}
	var decoded struct {
		ID             string `json:"id"`
		AgentConnected *bool  `json:"agentConnected"`
		ObservedAt     string `json:"observedAt"`
		LastKnownState string `json:"lastKnownState"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return obs, fmt.Errorf("%w: platform returned an unreadable target", ErrPlatformUnavailable)
	}
	obs.TargetID = targetID
	if decoded.AgentConnected != nil {
		obs.Authenticated = *decoded.AgentConnected
	}
	obs.State = decoded.LastKnownState
	if decoded.ObservedAt != "" {
		parsed, err := time.Parse(time.RFC3339, decoded.ObservedAt)
		if err != nil {
			return obs, fmt.Errorf("%w: platform returned an invalid observation time", ErrPlatformUnavailable)
		}
		obs.ObservedAt = parsed.UTC()
	}
	return obs, nil
}
