package db

import (
	"context"
	"errors"
	"net/http"
	"time"
)

type Pinger interface {
	PingContext(context.Context) error
}

// WaitForReady tolerates a dependency starting after the service. Each ping is
// bounded and the whole startup wait is limited to two minutes (or the caller's
// earlier deadline). Authentication and invalid database errors fail immediately.
func WaitForReady(ctx context.Context, database Pinger) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	delay := 100 * time.Millisecond
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		attempt, stop := context.WithTimeout(ctx, 3*time.Second)
		err := database.PingContext(attempt)
		stop()
		if err == nil {
			return nil
		}
		var state interface{ SQLState() string }
		if errors.As(err, &state) {
			code := state.SQLState()
			if code == "28P01" || code == "28000" || code == "3D000" {
				return err
			}
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		if delay < 2*time.Second {
			delay *= 2
			if delay > 2*time.Second {
				delay = 2 * time.Second
			}
		}
	}
}

// WithHealth keeps liveness independent from backing services. Readiness drops
// on database or additional dependency failure without exposing error details.
// Business routes and their authentication continue through the original handler.
func WithHealth(next http.Handler, database Pinger, dependencies ...func() bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/livez" && r.URL.Path != "/readyz" {
			next.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path == "/readyz" {
			ctx, cancel := context.WithTimeout(r.Context(), time.Second)
			defer cancel()
			if database.PingContext(ctx) != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			for _, ready := range dependencies {
				if !ready() {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
			}
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
