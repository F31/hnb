package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/F31/hnb/cmd/platform-api/internal/store"
)

// AppError represents a structured application error.
type AppError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	HTTPStatus int    `json:"-"`
}

// Error returns the error message.
func (e *AppError) Error() string {
	return e.Message
}

// ErrorHandler wraps http.Handler to handle errors uniformly.
func ErrorHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		// Use a deadline for long-running operations
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		r = r.WithContext(ctx)

		// Recover from panics
		defer func() {
			if rv := recover(); rv != nil {
				log.Printf("panic in %s %s: %v", r.Method, r.URL.Path, rv)
				writeJSON(w, http.StatusInternalServerError, map[string]string{
					"error":   "internal_server_error",
					"message": "An unexpected error occurred",
				})
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// WriteAppError writes an AppError response with appropriate status code.
func WriteAppError(w http.ResponseWriter, err error) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		writeJSON(w, appErr.HTTPStatus, map[string]string{
			"code":    appErr.Code,
			"message": appErr.Message,
		})
		return
	}

	// Map known store errors to appropriate codes
	if errors.Is(err, store.ErrClusterNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"code":    "cluster_not_found",
			"message": "Cluster not found",
		})
		return
	}
	if errors.Is(err, store.ErrTenantMismatch) {
		writeJSON(w, http.StatusForbidden, map[string]string{
			"code":    "tenant_mismatch",
			"message": "Tenant mismatch",
		})
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"code":    "not_found",
			"message": "Not found",
		})
		return
	}
	if errors.Is(err, store.ErrInvalidState) {
		writeJSON(w, http.StatusConflict, map[string]string{
			"code":    "invalid_state",
			"message": "Operation not allowed in current state",
		})
		return
	}

	// Default: internal server error
	log.Printf("unexpected error: %v", err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{
		"code":    "internal_error",
		"message": "An internal error occurred",
	})
}

// Helper: writeJSON writes JSON with appropriate content-type and status.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
