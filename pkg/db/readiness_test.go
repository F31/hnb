package db

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type pingFunc func(context.Context) error

func (f pingFunc) PingContext(ctx context.Context) error { return f(ctx) }

type stateError string

func (e stateError) Error() string    { return "invalid credentials" }
func (e stateError) SQLState() string { return string(e) }

func TestWaitForReadyRecoversAndHonorsCancellation(t *testing.T) {
	attempts := 0
	if err := WaitForReady(context.Background(), pingFunc(func(ctx context.Context) error {
		attempts++
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("ping is not bounded")
		}
		if attempts < 2 {
			return errors.New("connection refused")
		}
		return nil
	})); err != nil || attempts != 2 {
		t.Fatalf("attempts=%d err=%v", attempts, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := WaitForReady(ctx, pingFunc(func(context.Context) error {
		return errors.New("offline")
	})); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v", err)
	}
}

func TestWaitForReadyDoesNotRetryInvalidCredentials(t *testing.T) {
	for _, code := range []stateError{"28P01", "28000", "3D000"} {
		attempts := 0
		err := WaitForReady(context.Background(), pingFunc(func(context.Context) error {
			attempts++
			return code
		}))
		if err != code || attempts != 1 {
			t.Fatalf("attempts=%d err=%v", attempts, err)
		}
	}
}

func TestHealthDoesNotRestartForDependencyFailureOrBypassBusinessAuth(t *testing.T) {
	dbErr := errors.New("database-password-must-not-leak")
	connected := false
	h := WithHealth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}), pingFunc(func(context.Context) error { return dbErr }), func() bool { return connected })
	check := func(method, path string, want int) {
		t.Helper()
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, path, nil))
		if w.Code != want {
			t.Fatalf("%s %s: %d", method, path, w.Code)
		}
		if w.Body.Len() != 0 {
			t.Fatal("health exposes error details")
		}
	}
	check("GET", "/livez", 204)
	check("GET", "/readyz", 503)
	dbErr = nil
	check("GET", "/readyz", 503)
	connected = true
	check("GET", "/readyz", 204)
	check("GET", "/api/v1/tenants", 401)
	check("POST", "/readyz", 405)
}
