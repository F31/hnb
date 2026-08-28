package observer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/F31/hnb/pkg/iam"
)

// IdentityVerifier extracts the tenant-bound observer identity from an HTTP
// request. The projector treats this identity as authoritative over any
// payload fields.
type IdentityVerifier interface {
	VerifyObserverIdentity(context.Context, string) (Identity, error)
}

// ObserverTokenIdentityVerifier verifies observer identity JWTs issued by the
// identity service (pkg/iam ObserverTokenVerifier).
type ObserverTokenIdentityVerifier struct {
	verifier *iam.ObserverTokenVerifier
}

func NewObserverTokenIdentityVerifier(verifier *iam.ObserverTokenVerifier) *ObserverTokenIdentityVerifier {
	return &ObserverTokenIdentityVerifier{verifier: verifier}
}

func (v *ObserverTokenIdentityVerifier) VerifyObserverIdentity(ctx context.Context, bearer string) (Identity, error) {
	claims, err := v.verifier.Verify(ctx, bearer)
	if err != nil {
		return Identity{}, err
	}
	return Identity{
		TenantID:     claims.Identity.TenantID,
		TargetID:     claims.Identity.TargetID,
		TargetKind:   claims.Identity.TargetKind,
		ObserverID:   claims.Identity.ObserverID,
		ObserverKind: claims.Identity.ObserverKind,
	}, nil
}

type IngestHandler struct {
	projector *Projector
	verifier  IdentityVerifier
}

func NewIngestHandler(projector *Projector, verifier IdentityVerifier) *IngestHandler {
	return &IngestHandler{projector: projector, verifier: verifier}
}

// Routes returns the HTTP routes for observation ingest and source-reset.
func (h *IngestHandler) Routes() (string, http.HandlerFunc, string, http.HandlerFunc) {
	return "/v1/observations", h.handleObservation, "/v1/observations/reset", h.handleReset
}

// ServeHTTP dispatches observer ingest routes by path. Observer tokens
// self-authenticate; the caller (platform-api) routes these paths here before
// the browser/service access-token middleware.
func (h *IngestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeProblem(w, http.StatusMethodNotAllowed, "method_not_allowed", "only POST is allowed")
		return
	}
	switch r.URL.Path {
	case "/v1/observations":
		h.handleObservation(w, r)
	case "/v1/observations/reset":
		h.handleReset(w, r)
	default:
		writeProblem(w, http.StatusNotFound, "not_found", "unknown observer route")
	}
}

func (h *IngestHandler) handleObservation(w http.ResponseWriter, r *http.Request) {
	identity, err := h.identity(r)
	if err != nil {
		writeProblem(w, http.StatusUnauthorized, "observer-identity-denied", err.Error())
		return
	}
	payload, err := readBoundedBody(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	if err := h.projector.Accept(r.Context(), identity, payload); err != nil {
		switch {
		case errors.Is(err, ErrReplay):
			writeJSON(w, http.StatusOK, map[string]string{"status": "replayed"})
		case errors.Is(err, ErrGap):
			writeProblem(w, http.StatusConflict, "sequence_gap", err.Error())
		case errors.Is(err, ErrFenced):
			writeProblem(w, http.StatusConflict, "observer_fenced", err.Error())
		default:
			writeProblem(w, http.StatusUnprocessableEntity, "observation_rejected", err.Error())
		}
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

func (h *IngestHandler) handleReset(w http.ResponseWriter, r *http.Request) {
	identity, err := h.identity(r)
	if err != nil {
		writeProblem(w, http.StatusUnauthorized, "observer-identity-denied", err.Error())
		return
	}
	payload, err := readBoundedBody(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_payload", err.Error())
		return
	}
	if err := h.projector.ApplyReset(r.Context(), identity, payload); err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "source_reset_rejected", err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "reset_accepted"})
}

func (h *IngestHandler) identity(r *http.Request) (Identity, error) {
	auth := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(auth) <= len(prefix) || auth[:len(prefix)] != prefix {
		return Identity{}, fmt.Errorf("missing observer bearer token")
	}
	return h.verifier.VerifyObserverIdentity(r.Context(), auth[len(prefix):])
}

func readBoundedBody(r *http.Request) ([]byte, error) {
	r.Body = http.MaxBytesReader(nil, r.Body, MaxObservationPayload)
	data, err := io.ReadAll(io.LimitReader(r.Body, MaxObservationPayload+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxObservationPayload {
		return nil, fmt.Errorf("payload exceeds %d bytes", MaxObservationPayload)
	}
	return data, nil
}

type problem struct {
	Type   string `json:"type"`
	Status int    `json:"status"`
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`
}

func writeProblem(w http.ResponseWriter, status int, code, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(problem{Type: code, Status: status, Title: code, Detail: detail})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

var _ = time.Now
