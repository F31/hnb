package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/F31/hnb/cmd/apiserver/internal/response"
	"github.com/F31/hnb/pkg/iam"
)

const maxServiceIdentityBodyBytes = 16 << 10

// ServiceIdentityHandler exposes the restricted service identity lifecycle:
// registration, short-lived issuance, renewal, and revocation. Credentials are
// returned only to the caller; they are never written to the audit body.
type ServiceIdentityHandler struct {
	authority *iam.ServiceCredentialAuthority
	store     iam.ServiceIdentityStore
}

func NewServiceIdentityHandler(authority *iam.ServiceCredentialAuthority, store iam.ServiceIdentityStore) *ServiceIdentityHandler {
	return &ServiceIdentityHandler{authority: authority, store: store}
}

type serviceIdentityView struct {
	ID         string               `json:"id"`
	TenantID   string               `json:"tenantId"`
	SubjectID  string               `json:"subjectId"`
	Audience   string               `json:"audience"`
	Permission iam.ScopedPermission `json:"permission"`
	Status     string               `json:"status"`
	Version    int64                `json:"version"`
	CreatedAt  time.Time            `json:"createdAt"`
	UpdatedAt  time.Time            `json:"updatedAt"`
	RevokedAt  *time.Time           `json:"revokedAt,omitempty"`
}

type issuedServiceCredentialView struct {
	Credential        string               `json:"credential"`
	CredentialID      string               `json:"credentialId"`
	ServiceIdentityID string               `json:"serviceIdentityId"`
	SubjectID         string               `json:"subjectId"`
	TenantID          string               `json:"tenantId"`
	Audience          string               `json:"audience"`
	Permission        iam.ScopedPermission `json:"permission"`
	IssuedAt          string               `json:"issuedAt"`
	ExpiresAt         string               `json:"expiresAt"`
	ExpiresIn         int                  `json:"expiresIn"`
}

func (h *ServiceIdentityHandler) Create(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	var request struct {
		SubjectID  string               `json:"subjectId"`
		Audience   string               `json:"audience"`
		Permission iam.ScopedPermission `json:"permission"`
	}
	if !decodeServiceIdentityBody(w, r, &request) {
		return
	}
	identity, err := h.authority.CreateIdentity(r.Context(), trusted, iam.ServiceIdentityInput{
		TenantID: trusted.TenantID, SubjectID: request.SubjectID,
		Audience: request.Audience, Permission: request.Permission,
	})
	if err != nil {
		writeServiceIdentityError(w, err)
		return
	}
	response.Created(w, toServiceIdentityView(identity))
}

func (h *ServiceIdentityHandler) List(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	identities, err := h.store.ListServiceIdentities(r.Context(), trusted.TenantID)
	if err != nil {
		writeServiceIdentityError(w, err)
		return
	}
	views := make([]serviceIdentityView, 0, len(identities))
	for i := range identities {
		views = append(views, toServiceIdentityView(&identities[i]))
	}
	response.Success(w, views)
}

func (h *ServiceIdentityHandler) Get(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	identity, err := h.store.GetServiceIdentity(r.Context(), trusted.TenantID, r.PathValue("id"))
	if err != nil {
		writeServiceIdentityError(w, err)
		return
	}
	response.Success(w, toServiceIdentityView(identity))
}

func (h *ServiceIdentityHandler) Issue(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	var request struct {
		TTLSeconds int `json:"ttlSeconds"`
	}
	if !decodeServiceIdentityBody(w, r, &request) {
		return
	}
	if request.TTLSeconds < 0 {
		response.BadRequest(w, "ttlSeconds cannot be negative")
		return
	}
	issued, err := h.authority.Issue(r.Context(), trusted, r.PathValue("id"), time.Duration(request.TTLSeconds)*time.Second)
	if err != nil {
		writeServiceIdentityError(w, err)
		return
	}
	writeIssuedServiceCredential(w, issued)
}

// Renew is intentionally reachable without the user-token middleware: the
// presented service credential is verified against the owning-plane issuance
// record. It cannot broaden the registered scope.
func (h *ServiceIdentityHandler) Renew(w http.ResponseWriter, r *http.Request) {
	values := r.Header.Values("Authorization")
	if len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") {
		response.Unauthorized(w, "service credential required")
		return
	}
	token := strings.TrimPrefix(values[0], "Bearer ")
	if token == "" || strings.ContainsAny(token, " \t\r\n") {
		response.Unauthorized(w, "service credential required")
		return
	}
	issued, err := h.authority.Renew(r.Context(), token)
	if err != nil {
		writeServiceIdentityError(w, err)
		return
	}
	writeIssuedServiceCredential(w, issued)
}

func (h *ServiceIdentityHandler) RevokeIdentity(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	var request struct {
		Version int64  `json:"version"`
		Reason  string `json:"reason"`
	}
	if !decodeServiceIdentityBody(w, r, &request) {
		return
	}
	if request.Version <= 0 {
		response.BadRequest(w, "positive version is required")
		return
	}
	if _, err := h.authority.RevokeIdentity(r.Context(), trusted, r.PathValue("id"), request.Version, request.Reason); err != nil {
		writeServiceIdentityError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *ServiceIdentityHandler) RevokeCredential(w http.ResponseWriter, r *http.Request) {
	trusted, ok := iam.TrustedContextFrom(r.Context())
	if !ok {
		response.Unauthorized(w, "trusted context required")
		return
	}
	var request struct {
		Reason string `json:"reason"`
	}
	if !decodeServiceIdentityBody(w, r, &request) {
		return
	}
	if err := h.authority.RevokeCredential(r.Context(), trusted, r.PathValue("id"), r.PathValue("credentialId"), request.Reason); err != nil {
		writeServiceIdentityError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeServiceIdentityBody(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxServiceIdentityBodyBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		response.BadRequest(w, "invalid body")
		return false
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		response.BadRequest(w, "invalid body")
		return false
	}
	return true
}

func toServiceIdentityView(identity *iam.ServiceIdentityRecord) serviceIdentityView {
	return serviceIdentityView{
		ID: identity.ID, TenantID: identity.TenantID, SubjectID: identity.SubjectID,
		Audience: identity.Audience, Permission: identity.Permission, Status: identity.Status,
		Version: identity.Version, CreatedAt: identity.CreatedAt, UpdatedAt: identity.UpdatedAt,
		RevokedAt: identity.RevokedAt,
	}
}

func writeIssuedServiceCredential(w http.ResponseWriter, issued *iam.IssuedServiceCredential) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(issuedServiceCredentialView{
		Credential: issued.Credential, CredentialID: issued.CredentialID,
		ServiceIdentityID: issued.ServiceIdentityID, SubjectID: issued.SubjectID,
		TenantID: issued.TenantID, Audience: issued.Audience, Permission: issued.Permission,
		IssuedAt:  issued.IssuedAt.UTC().Format(time.RFC3339),
		ExpiresAt: issued.ExpiresAt.UTC().Format(time.RFC3339),
		ExpiresIn: int(issued.ExpiresAt.Sub(issued.IssuedAt).Seconds()),
	})
}

func writeServiceIdentityError(w http.ResponseWriter, err error) {
	var validation iam.ServiceIdentityValidationError
	switch {
	case errors.As(err, &validation):
		response.BadRequest(w, validation.Message)
	case errors.Is(err, iam.ErrServiceIdentityNotFound), errors.Is(err, iam.ErrServiceCredentialNotFound):
		response.NotFound(w, "service identity not found")
	case errors.Is(err, iam.ErrServiceIdentityExists):
		response.Error(w, http.StatusConflict, response.CodeConflict, "service identity already exists")
	case errors.Is(err, iam.ErrServiceIdentityVersionConflict), errors.Is(err, iam.ErrServiceCredentialConflict):
		response.Error(w, http.StatusConflict, response.CodeConflict, "service identity version conflict")
	case errors.Is(err, iam.ErrServiceIdentityRevoked):
		response.Error(w, http.StatusConflict, response.CodeConflict, "service identity is revoked")
	case errors.Is(err, iam.ErrServiceCredentialRejected), errors.Is(err, iam.ErrEnrollmentUnavailable):
		response.Unauthorized(w, "service credential rejected")
	case errors.Is(err, iam.ErrServiceCredentialUnavailable):
		response.ServiceUnavailable(w, "service credential unavailable")
	default:
		response.InternalError(w, "service identity operation failed")
	}
}
