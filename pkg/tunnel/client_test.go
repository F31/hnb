package tunnel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type staticTokenSource string

func (s staticTokenSource) Token(context.Context) (string, error) { return string(s), nil }

func TestAgentClientUsesAuthorizationHeaderWithoutURLToken(t *testing.T) {
	var requestURI string
	verified := false
	tunnelServer := NewTunnelServer(func(_ context.Context, token, tenantID, clusterID string) (time.Time, error) {
		verified = token == "service-token" && tenantID == "tenant-a" && clusterID == "cluster-a"
		return time.Now().Add(time.Minute), nil
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestURI = r.RequestURI
		tunnelServer.ServeHTTP(w, r)
	}))
	defer server.Close()

	client := NewAgentClient("ws"+strings.TrimPrefix(server.URL, "http"), staticTokenSource("service-token"), "tenant-a", "cluster-a")
	if err := client.Connect(); err != nil {
		t.Fatal(err)
	}
	client.Close()
	if !verified {
		t.Fatal("service identity verifier did not receive the expected bound scope")
	}
	if strings.Contains(requestURI, "token") || strings.Contains(requestURI, "service-token") || strings.Contains(requestURI, "?") {
		t.Fatalf("tunnel request URI exposed a token or query: %q", requestURI)
	}
}

func TestTunnelServerRejectsLegacyQueryTokenBeforeUpgrade(t *testing.T) {
	server := NewTunnelServer(func(context.Context, string, string, string) (time.Time, error) {
		return time.Now().Add(time.Minute), nil
	})
	request := httptest.NewRequest(http.MethodGet, "/tunnel?token=legacy", nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", response.Code)
	}
}

type rotatingTokenSource struct {
	mu     sync.Mutex
	tokens []string
	reads  int
}

func (s *rotatingTokenSource) Token(context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	token := s.tokens[s.reads]
	s.reads++
	return token, nil
}

func TestExpiredTunnelClosesAndReconnectReadsRotatedToken(t *testing.T) {
	var requestURIs []string
	var mu sync.Mutex
	tunnelServer := NewTunnelServer(func(_ context.Context, token, _, _ string) (time.Time, error) {
		if token == "short-lived" {
			return time.Now().Add(100 * time.Millisecond), nil
		}
		return time.Now().Add(time.Minute), nil
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestURIs = append(requestURIs, r.RequestURI)
		mu.Unlock()
		tunnelServer.ServeHTTP(w, r)
	}))
	defer server.Close()

	tokens := &rotatingTokenSource{tokens: []string{"short-lived", "rotated"}}
	client := NewAgentClient("ws"+strings.TrimPrefix(server.URL, "http"), tokens, "tenant-a", "cluster-a")
	if err := client.Connect(); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ReadMessage(); err == nil {
		t.Fatal("connection remained open after trusted token expiry")
	}
	client.Close()
	if err := client.Connect(); err != nil {
		t.Fatalf("reconnect with rotated token: %v", err)
	}
	client.Close()

	tokens.mu.Lock()
	reads := tokens.reads
	tokens.mu.Unlock()
	if reads != 2 {
		t.Fatalf("token file reads = %d, want 2", reads)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, requestURI := range requestURIs {
		if strings.Contains(requestURI, "token") || strings.Contains(requestURI, "short-lived") || strings.Contains(requestURI, "rotated") || strings.Contains(requestURI, "?") {
			t.Fatalf("tunnel request URI exposed token data: %q", requestURI)
		}
	}
}

func TestTunnelServerRoutesAgentResponseToProxyRequest(t *testing.T) {
	tunnelServer := NewTunnelServer(func(context.Context, string, string, string) (time.Time, error) {
		return time.Now().Add(time.Minute), nil
	})
	server := httptest.NewServer(tunnelServer)
	defer server.Close()

	client := NewAgentClient("ws"+strings.TrimPrefix(server.URL, "http"), staticTokenSource("service-token"), "tenant-a", "cluster-a")
	if err := client.Connect(); err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	requestHandled := make(chan error, 1)
	go func() {
		msg, err := client.ReadMessage()
		if err != nil {
			requestHandled <- err
			return
		}
		var request RequestPayload
		if err := json.Unmarshal(msg.Payload, &request); err != nil {
			requestHandled <- err
			return
		}
		requestHandled <- client.SendResponse(request.RequestID, http.StatusOK, map[string]string{"Content-Type": "text/plain"}, []byte("ok"))
	}()

	response, err := tunnelServer.ProxyRequest("cluster-a", &RequestPayload{RequestID: "request-a", Method: http.MethodGet, Path: "api/v1/pods"})
	if err != nil {
		t.Fatal(err)
	}
	if err := <-requestHandled; err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || string(response.Body) != "ok" {
		t.Fatalf("response = status %d body %q", response.StatusCode, response.Body)
	}
}
