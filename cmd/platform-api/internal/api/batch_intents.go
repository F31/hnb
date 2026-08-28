package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/F31/hnb/cmd/platform-api/internal/engine"
	"github.com/F31/hnb/cmd/platform-api/internal/store"
	"github.com/F31/hnb/pkg/iam"
	"github.com/google/uuid"
)

func uuidStr(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

func (s *Server) handleCreateRuntimeIntentBatch(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "trusted context required")
		return
	}
	if !s.authorize(w, r, "", "", "", "") {
		return
	}
	batchStore, ok := s.store.(store.BatchOperationStore)
	if !ok {
		writeError(w, http.StatusServiceUnavailable, "batch_unavailable", "batch operations are not configured")
		return
	}
	var req BatchDeleteRuntimeTargetsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid batch request")
		return
	}
	if len(req.TargetIDs) == 0 || len(req.TargetIDs) > 100 || strings.TrimSpace(req.IdempotencyKey) == "" || !uuidStr(req.CorrelationID) || req.CorrelationID != trusted.CorrelationID {
		writeError(w, http.StatusBadRequest, "invalid_request", "targetIds, idempotencyKey and correlationId are required")
		return
	}
	seen := map[string]bool{}
	for _, id := range req.TargetIDs {
		if !uuidStr(id) || seen[id] {
			writeError(w, http.StatusBadRequest, "invalid_request", "targetIds must be unique UUIDs")
			return
		}
		seen[id] = true
	}
	targets := make([]*store.RuntimeTarget, 0, len(req.TargetIDs))
	for _, id := range req.TargetIDs {
		target, err := s.store.GetRuntimeTarget(r.Context(), id, trusted.TenantID)
		if err != nil || (target.TargetType != "kubernetes" && target.TargetType != "edge_runtime") {
			writeError(w, http.StatusConflict, "batch_preflight_failed", "one or more targets cannot be unmanaged")
			return
		}
		targets = append(targets, target)
	}
	batch, created, err := batchStore.CreateOperationBatch(r.Context(), store.OperationBatch{TenantID: trusted.TenantID, Kind: "BatchDeleteRuntimeTargets", InitiatedBy: trusted.SubjectID, CorrelationID: req.CorrelationID, IdempotencyKey: req.IdempotencyKey, TargetIDs: req.TargetIDs})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "batch_create_failed", "could not create batch")
		return
	}
	if created {
		for ordinal, target := range targets {
			targetKind := "KubernetesTarget"
			if target.TargetType == "edge_runtime" {
				targetKind = "EdgeRuntimeTarget"
			}
			intent := &engine.RuntimeIntent{APIVersion: "hnb.io/v1", Kind: engine.IntentDeleteRuntimeTarget,
				Metadata: engine.IntentMetadata{IdempotencyKey: req.IdempotencyKey + ":" + target.ID, CorrelationID: req.CorrelationID},
				Spec:     engine.IntentSpec{TargetID: target.ID, TargetKind: targetKind, ExpectedVersion: target.ProjectionVersion, ScopeRef: "tenant:" + trusted.TenantID}}
			plan, planErr := s.engine.ProcessWithContext(r.Context(), intent, trusted.TenantID)
			if planErr != nil {
				continue
			}
			op, _, submitErr := s.store.SubmitIntent(r.Context(), store.IntentSubmitCommand{Intent: intent, ExecutionPlan: plan, TenantID: trusted.TenantID, SubjectID: trusted.SubjectID, InitiatedBy: trusted.SubjectID, CorrelationID: req.CorrelationID, RuntimeTargetID: target.ID, ExpectedTargetVersion: target.ProjectionVersion, CommitmentAction: string(iam.ActionDelete)})
			if submitErr == nil {
				_ = batchStore.AttachOperationBatchChild(r.Context(), batch.ID, op.ID, target.ID, ordinal)
			}
		}
		batch, _ = batchStore.RefreshOperationBatchStatus(r.Context(), batch.ID, trusted.TenantID)
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"batch": batch, "replayed": !created})
}
