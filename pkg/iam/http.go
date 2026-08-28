package iam

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"regexp"
	"strings"
)

type AccessAuthenticator interface {
	Authenticate(context.Context, string, string, string) (TrustedContext, error)
}

var (
	correlationPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	traceparentPattern = regexp.MustCompile(`^[0-9a-f]{2}-[0-9a-f]{32}-[0-9a-f]{16}-[0-9a-f]{2}$`)
)

func TrustedHTTPMiddleware(authenticator AccessAuthenticator, bypassPaths ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			SanitizeIdentityHeaders(r.Header)
			if contains(bypassPaths, r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			values := r.Header.Values("Authorization")
			if len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			token := strings.TrimPrefix(values[0], "Bearer ")
			if token == "" || strings.ContainsAny(token, " \t\r\n") {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			correlationID := strings.ToLower(r.Header.Get("X-Correlation-ID"))
			if !correlationPattern.MatchString(correlationID) {
				correlationID = newCorrelationID()
			}
			r.Header.Set("X-Correlation-ID", correlationID)
			traceparent := strings.ToLower(r.Header.Get("traceparent"))
			if traceparent != "" && !traceparentPattern.MatchString(traceparent) {
				r.Header.Del("traceparent")
				traceparent = ""
			}
			trusted, err := authenticator.Authenticate(r.Context(), token, correlationID, traceparent)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			r.Header.Del("Authorization")
			next.ServeHTTP(w, r.WithContext(WithTrustedContext(r.Context(), trusted)))
		})
	}
}

func SanitizeIdentityHeaders(header http.Header) {
	for name := range header {
		lower := strings.ToLower(name)
		if !strings.HasPrefix(lower, "x-") || lower == "x-correlation-id" {
			continue
		}
		for _, term := range []string{"tenant", "user", "subject", "identity", "membership", "actor", "workspace", "role", "permission", "approval"} {
			if strings.Contains(lower, term) {
				header.Del(name)
				break
			}
		}
	}
}

func newCorrelationID() string {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(value)
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:]
}
