package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/F31/hnb/pkg/audit"
)

type auditCapture struct {
	events chan *audit.Event
}

func (c *auditCapture) Create(event *audit.Event) error {
	c.events <- event
	return nil
}

func TestAuditOmitsCredentialEndpointBodies(t *testing.T) {
	for _, path := range []string{"/api/v1/auth/login", "/api/v1/auth/refresh"} {
		t.Run(path, func(t *testing.T) {
			capture := &auditCapture{events: make(chan *audit.Event, 1)}
			mw := NewAuditMW(capture)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"password":"secret","refresh_token":"raw"}`))
			ctx := &Context{Request: request, Response: recorder}
			mw.Handle(ctx, func() { _, _ = ctx.Response.Write([]byte(`{"access_token":"raw"}`)) })
			select {
			case event := <-capture.events:
				if event.RequestBody != "" || event.ResponseBody != "" {
					t.Fatalf("credential bodies were audited: %#v", event)
				}
			case <-time.After(time.Second):
				t.Fatal("audit event was not captured")
			}
		})
	}
}

func TestRedactJSONBodyRecursively(t *testing.T) {
	body := []byte(`{"name":"safe","nested":{"password":"p","access_token":"a"},"items":[{"private-key":"k","secret":"s"}]}`)
	redacted := redactJSONBody(body)
	for _, secret := range []string{`"p"`, `"a"`, `"k"`, `"s"`} {
		if bytes.Contains([]byte(redacted), []byte(secret)) {
			t.Fatalf("secret %s remains in %s", secret, redacted)
		}
	}
	if !strings.Contains(redacted, `"name":"safe"`) || strings.Count(redacted, "[REDACTED]") != 4 {
		t.Fatalf("unexpected redaction: %s", redacted)
	}
}

func TestStorageBackendAuditClassifiesAndRedactsSecretReference(t *testing.T) {
	capture := &auditCapture{events: make(chan *audit.Event, 1)}
	mw := NewAuditMW(capture)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/storage/backends", strings.NewReader(`{"displayName":"safe","secretReference":{"name":"credential-name"}}`))
	ctx := &Context{Request: request, Response: recorder, TenantID: "tenant-a", UserID: "subject-a"}
	mw.Handle(ctx, func() { ctx.Response.WriteHeader(http.StatusCreated) })
	select {
	case event := <-capture.events:
		if event.ResourceType != "storageBackend" || strings.Contains(event.RequestBody, "credential-name") || !strings.Contains(event.RequestBody, "[REDACTED]") {
			t.Fatalf("unexpected storage audit event: %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("audit event was not captured")
	}
}
