package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func base64Std(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

// testSecretCipher is a deterministic in-memory cipher for exercising the

type testSecretCipher struct {
	lastSealed string
	values     map[string][]byte
}

func (c *testSecretCipher) Encrypt(plaintext []byte) (string, error) {
	c.lastSealed = "sealed:" + string(plaintext)
	if c.values == nil {
		c.values = map[string][]byte{}
	}
	c.values[c.lastSealed] = append([]byte(nil), plaintext...)
	return c.lastSealed, nil
}

func (c *testSecretCipher) Decrypt(sealed string) ([]byte, error) {
	if v, ok := c.values[sealed]; ok {
		return append([]byte(nil), v...), nil
	}
	return nil, errors.New("testSecretCipher: unknown sealed value")
}

func newSecretTestServer(t *testing.T) (*Server, *fakeStore, *testSecretCipher) {
	t.Helper()
	st := newFakeStore()
	srv := NewServer(st, testAuthenticator{}, testPermissionResolver{policy: "default:2", permissions: platformTestPermissions("tenant-a")})
	cipher := &testSecretCipher{}
	srv.ConfigureKMS(cipher)
	return srv, st, cipher
}

func TestRegisterSecretSuccess(t *testing.T) {
	srv, st, cipher := newSecretTestServer(t)
	kc := "apiVersion: v1\nkind: Config\nclusters: []\nusers: []\n"
	body := `{"purpose":"kubeconfig","scope":"tenant-a","name":"cluster-a","value":"` + base64Std([]byte(kc)) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/secrets:register", strings.NewReader(body))
	setTestSecretDelegation(t, srv, req)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var resp registerSecretResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Provider != "local-aes" || resp.Scope != "tenant-a" || resp.Name != "cluster-a" || resp.Version != "1" || resp.Purpose != "kubeconfig" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if len(st.audits) == 0 {
		t.Fatal("expected a security audit record for secret registration")
	}
	if st.secrets == nil || st.secrets["tenant-a/cluster-a"] == nil {
		t.Fatal("expected the secret reference to be recorded in the store")
	}
	if cipher.lastSealed == "" {
		t.Fatal("expected the cipher to encrypt the value")
	}
}

func TestRegisterSecretRejectsInvalidPurpose(t *testing.T) {
	srv, _, _ := newSecretTestServer(t)
	for _, purpose := range []string{"", "password", "Kubeconfig"} {
		body := `{"purpose":"` + purpose + `","scope":"tenant-a","name":"cluster-a","value":"ZGF0YQ=="}`
		req := httptest.NewRequest(http.MethodPost, "/v1/secrets:register", strings.NewReader(body))
		setTestSecretDelegation(t, srv, req)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("purpose=%q status = %d, body %s", purpose, rec.Code, rec.Body)
		}
	}
}

func TestRegisterSecretRejectsMalformed(t *testing.T) {
	srv, _, _ := newSecretTestServer(t)
	cases := []string{
		`{"purpose":"kubeconfig","scope":"","name":"cluster-a","value":"ZGF0YQ=="}`,
		`{"purpose":"kubeconfig","scope":"tenant-a","name":"","value":"ZGF0YQ=="}`,
		`{"purpose":"kubeconfig","scope":"tenant-a","name":"cluster-a","value":""}`,
		`{"purpose":"kubeconfig","scope":"tenant-a","name":"cluster-a","value":"!!!!"}`,
	}
	for _, body := range cases {
		req := httptest.NewRequest(http.MethodPost, "/v1/secrets:register", strings.NewReader(body))
		setTestSecretDelegation(t, srv, req)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("body=%s status = %d, body %s", body, rec.Code, rec.Body)
		}
	}
}

func TestRegisterSecretRejectsInvalidKubeconfigShape(t *testing.T) {
	srv, _, _ := newSecretTestServer(t)
	// "not a kubeconfig at all"
	body := `{"purpose":"kubeconfig","scope":"tenant-a","name":"cluster-a","value":"` + base64Std([]byte("hello world")) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/secrets:register", strings.NewReader(body))
	setTestSecretDelegation(t, srv, req)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
}

func TestRegisterSecretKubeconfigAccepted(t *testing.T) {
	srv, _, _ := newSecretTestServer(t)
	kc := "apiVersion: v1\nkind: Config\nclusters: []\nusers: []\n"
	body := `{"purpose":"kubeconfig","scope":"tenant-a","name":"cluster-a","value":"` + base64Std([]byte(kc)) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/secrets:register", strings.NewReader(body))
	setTestSecretDelegation(t, srv, req)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
}

func TestRegisterSecretRequiresDelegation(t *testing.T) {
	srv, _, _ := newSecretTestServer(t)
	body := `{"purpose":"kubeconfig","scope":"tenant-a","name":"cluster-a","value":"ZGF0YQ=="}`
	// No delegation at all -> 401 from the delegation guard.
	req := httptest.NewRequest(http.MethodPost, "/v1/secrets:register", strings.NewReader(body))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no-delegation status = %d, body %s", rec.Code, rec.Body)
	}
	// A plain access token is not accepted on the delegation-only route.
	req2 := httptest.NewRequest(http.MethodPost, "/v1/secrets:register", strings.NewReader(body))
	req2.Header.Set("Authorization", "Bearer valid-token")
	rec2 := httptest.NewRecorder()
	srv.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("user-token status = %d, body %s", rec2.Code, rec2.Body)
	}
}

func TestRegisterSecretDeniedWithoutPermission(t *testing.T) {
	st := newFakeStore()
	srv := NewServer(st, noPermissionAuthenticator{})
	body := `{"purpose":"kubeconfig","scope":"tenant-a","name":"cluster-a","value":"ZGF0YQ=="}`
	req := httptest.NewRequest(http.MethodPost, "/v1/secrets:register", strings.NewReader(body))
	setTestSecretDelegation(t, srv, req)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("no-permission status = %d, body %s", rec.Code, rec.Body)
	}
	if st.calls != 0 {
		t.Fatalf("store should not be touched before permission check, calls=%d", st.calls)
	}
}
