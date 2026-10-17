package api

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/F31/hnb/cmd/platform-api/internal/engine"
	"github.com/F31/hnb/cmd/platform-api/internal/store"
	"github.com/F31/hnb/pkg/core"
	"github.com/F31/hnb/pkg/iam"
)

// LoadConformanceKeys reads only trusted public keys (signer ID -> hex key).
func LoadConformanceKeys(path string) (map[string]ed25519.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var encoded map[string]string
	if err := json.Unmarshal(data, &encoded); err != nil {
		return nil, err
	}
	if len(encoded) == 0 {
		return nil, errors.New("conformance trust file is empty")
	}
	keys := make(map[string]ed25519.PublicKey, len(encoded))
	for id, value := range encoded {
		key, err := hex.DecodeString(value)
		if err != nil || len(key) != ed25519.PublicKeySize || id == "" {
			return nil, errors.New("invalid conformance public key")
		}
		keys[id] = ed25519.PublicKey(key)
	}
	return keys, nil
}

func (s *Server) ConfigureConformanceKeys(keys map[string]ed25519.PublicKey) {
	s.conformanceKeys = make(map[string]ed25519.PublicKey, len(keys))
	for id, key := range keys {
		s.conformanceKeys[id] = append(ed25519.PublicKey(nil), key...)
	}
}

func (s *Server) handleRegisterConformance(w http.ResponseWriter, r *http.Request) {
	providerID := r.PathValue("id")
	if s.permissionResolver != nil {
		trusted, _ := iam.TrustedContextFrom(r.Context())
		policy, permissions, err := s.permissionResolver.ResolvePermissions(r.Context(), trusted.SubjectID, trusted.MembershipID, trusted.TenantID)
		if err != nil {
			writeError(w, http.StatusForbidden, "permission_denied", "permission denied")
			return
		}
		trusted.PolicyVersion, trusted.ScopedPermissions = policy, permissions
		r = r.WithContext(iam.WithTrustedContext(r.Context(), trusted))
	}
	if !s.authorize(w, r, providerID, "", "", "") {
		return
	}
	if len(s.conformanceKeys) == 0 {
		writeError(w, http.StatusServiceUnavailable, "conformance_unavailable", "conformance trust is not configured")
		return
	}
	var signed core.SignedConformanceReport
	if err := decodeJSON(w, r, &signed); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_conformance", "invalid conformance document")
		return
	}
	if signed.Report.ProviderID != providerID {
		writeError(w, http.StatusBadRequest, "invalid_conformance", "provider identity mismatch")
		return
	}
	if err := signed.Verify(s.conformanceKeys, time.Now().UTC()); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_conformance", err.Error())
		return
	}
	decision, err := engine.DefaultCompatibilityMatrix().Evaluate(engine.IntentImportRuntimeTarget, signed.Report.TargetKind)
	if err != nil || decision.ProviderID != providerID || decision.MatrixVersion != signed.Report.MatrixVersion {
		writeError(w, http.StatusConflict, "conformance_matrix_changed", "matrix identity mismatch")
		return
	}
	if err := s.store.RegisterConformance(r.Context(), signed); err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			writeError(w, http.StatusNotFound, "not_found", "provider is not registered")
		case errors.Is(err, store.ErrConformanceIdentityChanged):
			writeError(w, http.StatusConflict, "conformance_identity_changed", "provider manifest identity changed")
		default:
			writeError(w, http.StatusInternalServerError, "conformance_save_failed", "conformance registration failed")
		}
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"providerId": providerID,
		"evidenceRef": signed.EvidenceDigest(), "scope": signed.Report.TargetKind + "/" + signed.Report.Action})
}
