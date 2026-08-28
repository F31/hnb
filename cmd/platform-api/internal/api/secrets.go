package api

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/F31/hnb/cmd/platform-api/internal/store"
	"github.com/F31/hnb/pkg/iam"
)

// secretCipher abstracts the AES-256-GCM master-key cipher used to seal and
// open secret payloads at rest.
type secretCipher interface {
	Encrypt(plaintext []byte) (string, error)
	Decrypt(sealed string) ([]byte, error)
}

// ConfigureKMS wires the master-key cipher used by the secret registration
// endpoint. When unset the endpoint fails closed.
func (s *Server) ConfigureKMS(cipher secretCipher) {
	s.secretCipher = cipher
}

const (
	maxSecretValueBytes = 1 << 20 // 1 MiB base64-encoded secret value
	maxSecretNameLen    = 128
)

var secretNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)
var secretScopePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:\-]{0,127}$`)

// secretPurposes enumerates the purposes a console-registered secret may
// serve. Each purpose pairs with the lifecycle provider allowed to resolve it.
var secretPurposes = map[string]string{
	"kubeconfig":       "runtime-target.lifecycle.kubernetes",
	"cloudcore-client": "runtime-target.lifecycle.edge",
}

type registerSecretRequest struct {
	Purpose string `json:"purpose"`
	Scope   string `json:"scope"`
	Name    string `json:"name"`
	Value   string `json:"value"` // base64-encoded raw secret value
}

type registerSecretResponse struct {
	APIVersion string `json:"apiVersion"`
	Provider   string `json:"provider"`
	Scope      string `json:"scope"`
	Name       string `json:"name"`
	Version    string `json:"version"`
	Purpose    string `json:"purpose"`
}

func (s *Server) handleRegisterSecret(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "trusted context is required")
		return
	}
	delegation, ok := iam.DelegationClaimsFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid_delegation", "trusted service delegation is required")
		return
	}
	if delegation.ActorSubject != trusted.SubjectID || delegation.MembershipID != trusted.MembershipID ||
		delegation.TenantID != trusted.TenantID ||
		delegation.Scope.ResourceKind != string(iam.ResourceSecret) || delegation.Action != iam.ActionCreate ||
		delegation.CorrelationID != trusted.CorrelationID ||
		delegation.CorrelationID != strings.TrimSpace(r.Header.Get("X-Correlation-ID")) {
		writeError(w, http.StatusUnauthorized, "delegation_evidence_mismatch", "delegation evidence does not match the request")
		return
	}
	var allowed bool
	trusted, allowed = s.authorizeSecretCommitment(w, r, trusted)
	if !allowed {
		return
	}
	if s.secretCipher == nil {
		writeError(w, http.StatusServiceUnavailable, "secret-store-unavailable", "secret storage is unavailable")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBodyBytes+64))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "failed to read request body")
		return
	}
	var req registerSecretRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "request body must be valid JSON")
		return
	}
	if err := validateRegisterSecretRequest(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(req.Value))
	if err != nil || len(raw) == 0 || len(raw) > maxSecretValueBytes {
		writeError(w, http.StatusBadRequest, "invalid_request", "value must be a non-empty base64 string (max 1 MiB decoded)")
		return
	}
	if req.Purpose == "kubeconfig" && !looksLikeKubeConfig(raw) {
		writeError(w, http.StatusBadRequest, "invalid_request", "value does not look like a kubeconfig (YAML or base64 data)")
		return
	}
	allowedProvider := secretPurposes[req.Purpose]
	sealed, err := s.secretCipher.Encrypt(raw)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "secret-encryption-failed", "secret encryption failed")
		return
	}
	metadata, err := s.store.RegisterSecretReference(r.Context(), store.RegisterSecretReferenceRequest{
		TenantID:                   trusted.TenantID,
		Scope:                      req.Scope,
		Name:                       req.Name,
		Purpose:                    req.Purpose,
		AllowedLifecycleProviderID: allowedProvider,
		EncryptedValue:             sealed,
		Algorithm:                  "AES-256-GCM",
		SubjectID:                  trusted.SubjectID,
	})
	if err != nil {
		log.Printf("platform-api: register secret reference: %v", err)
		writeError(w, http.StatusInternalServerError, "internal", "internal server error")
		return
	}
	if err := s.store.AppendSecurityAudit(r.Context(), store.SecurityAuditRecord{
		TenantID: trusted.TenantID, SubjectID: trusted.SubjectID, EventType: "secret_registered",
		Decision: "allow", ReasonCode: "secret-registered", Action: string(iam.ActionCreate), ResourceID: req.Name,
		CorrelationID: trusted.CorrelationID, TraceID: r.Header.Get("X-Trace-Id"), Outcome: "ok",
		Detail: map[string]any{"purpose": req.Purpose, "scope": req.Scope},
	}); err != nil {
		log.Printf("platform-api: security audit for secret registration failed; continuing: %v", err)
	}
	writeJSON(w, http.StatusCreated, registerSecretResponse{
		APIVersion: "hnb.io/v1",
		Provider:   metadata.Provider,
		Scope:      metadata.Scope,
		Name:       metadata.Name,
		Version:    metadata.Version,
		Purpose:    metadata.Purpose,
	})
}

func (s *Server) authorizeSecretCommitment(w http.ResponseWriter, r *http.Request, trusted iam.TrustedContext) (iam.TrustedContext, bool) {
	if s.permissionResolver != nil {
		policyVersion, permissions, err := s.permissionResolver.ResolvePermissions(r.Context(), trusted.SubjectID, trusted.MembershipID, trusted.TenantID)
		if err != nil {
			writeError(w, http.StatusForbidden, "permission_denied", "permission denied")
			return trusted, false
		}
		trusted.PolicyVersion, trusted.ScopedPermissions = policyVersion, permissions
	}
	decision := s.authz.Evaluate(trusted, iam.AuthorizationRequest{
		SubjectID: trusted.SubjectID, TenantID: trusted.TenantID, ResourceKind: string(iam.ResourceSecret),
		Action: iam.ActionCreate, ProjectID: trusted.ProjectID, EnvironmentID: trusted.EnvironmentID, NamespaceID: trusted.NamespaceID,
	})
	if !decision.Allowed {
		writeError(w, http.StatusForbidden, decision.ReasonCode, "permission denied")
		return trusted, false
	}
	return trusted, true
}

func validateRegisterSecretRequest(req *registerSecretRequest) error {
	switch req.Purpose {
	case "kubeconfig", "cloudcore-client":
	default:
		return &badSecretField{Field: "purpose", Reason: "purpose must be kubeconfig or cloudcore-client"}
	}
	if len(req.Scope) == 0 || len(req.Scope) > maxSecretNameLen || !secretScopePattern.MatchString(req.Scope) {
		return &badSecretField{Field: "scope", Reason: "scope must be a non-empty identifier"}
	}
	if len(req.Name) == 0 || len(req.Name) > maxSecretNameLen || !secretNamePattern.MatchString(req.Name) {
		return &badSecretField{Field: "name", Reason: "name must be a non-empty identifier"}
	}
	if req.Value == "" {
		return &badSecretField{Field: "value", Reason: "value is required"}
	}
	return nil
}

type badSecretField struct {
	Field  string
	Reason string
}

func (e *badSecretField) Error() string { return e.Field + ": " + e.Reason }

// looksLikeKubeConfig performs a lightweight structural check that the payload
// is either a kubeconfig YAML document or a base64-encoded one. It never logs
// or returns the payload content.
func looksLikeKubeConfig(raw []byte) bool {
	trimmed := trimLeftSpace(raw)
	if len(trimmed) == 0 {
		return false
	}
	if isBase64ish(trimmed) {
		decoded, err := base64.StdEncoding.DecodeString(string(trimmed))
		if err != nil {
			return false
		}
		return containsAll(decoded, []byte("apiVersion"), []byte("clusters"))
	}
	if strings.HasPrefix(string(trimmed), "---") || strings.HasPrefix(string(trimmed), "apiVersion") {
		return containsAll(trimmed, []byte("apiVersion"), []byte("clusters"))
	}
	return false
}

func trimLeftSpace(b []byte) []byte {
	i := 0
	for i < len(b) && (b[i] == ' ' || b[i] == '\t' || b[i] == '\n' || b[i] == '\r') {
		i++
	}
	return b[i:]
}

func isBase64ish(b []byte) bool {
	if len(b) < 16 || len(b)%4 != 0 {
		return false
	}
	for _, c := range b {
		if c != '=' && (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '+' && c != '/' {
			return false
		}
	}
	return true
}

func containsAll(b []byte, needles ...[]byte) bool {
	for _, n := range needles {
		found := false
		for i := 0; i+len(n) <= len(b); i++ {
			if string(b[i:i+len(n)]) == string(n) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
