package toolkit

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// kubeconfigTLSFixture returns a TLS server plus the certificate-authority-data
// that verifies it, suitable for embedding in a kubeconfig.
func kubeconfigTLSFixture(t *testing.T) (server *httptest.Server, caData string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "kubeconfig-fixture"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok") }))
	srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}, MinVersion: tls.VersionTLS12}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return srv, base64.StdEncoding.EncodeToString(caPEM)
}

func writeKubeconfig(t *testing.T, dir string, clusterName, server, caData string, insecure bool) string {
	t.Helper()
	var caField, serverField string
	if insecure {
		caField = "    insecure-skip-tls-verify: true"
	} else if caData != "" {
		caField = fmt.Sprintf("    certificate-authority-data: %s", caData)
	}
	if server != "" {
		serverField = fmt.Sprintf("    server: %s", server)
	}
	doc := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: %s
  cluster:
%s
%s
contexts:
- name: test
  context:
    cluster: %s
    user: test
current-context: test
users:
- name: test
  user:
    token: never-read
`, clusterName, serverField, caField, clusterName)
	path := filepath.Join(dir, "kubeconfig.yaml")
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestKubeconfigProbeFileAndServerReference(t *testing.T) {
	srv, caData := kubeconfigTLSFixture(t)
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "https://"))
	dir := t.TempDir()

	valid := writeKubeconfig(t, dir, "c0", "https://127.0.0.1:"+port, caData, false)
	checks := ProbeExistingKubeconfig(context.Background(), valid)
	if len(checks) < 2 || checks[0].Status != StatusPass || checks[1].Status != StatusPass {
		t.Fatalf("valid kubeconfig did not pass file/server checks: %+v", checks)
	}

	missing := filepath.Join(dir, "does-not-exist.yaml")
	checks = ProbeExistingKubeconfig(context.Background(), missing)
	if len(checks) != 1 || checks[0].ReasonCode != "KUBECONFIG_UNAVAILABLE" {
		t.Fatalf("missing kubeconfig not reported: %+v", checks)
	}

	httpRef := writeKubeconfig(t, dir, "c1", "http://127.0.0.1:80", caData, false)
	checks = ProbeExistingKubeconfig(context.Background(), httpRef)
	if checks[1].ReasonCode != "API_SERVER_REFERENCE_INVALID" || !strings.Contains(checks[1].Actual, "https") {
		t.Fatalf("http server reference accepted: %+v", checks)
	}

	credRef := writeKubeconfig(t, dir, "c2", "https://user:secret@127.0.0.1:"+port, caData, false)
	checks = ProbeExistingKubeconfig(context.Background(), credRef)
	if checks[1].ReasonCode != "API_SERVER_REFERENCE_INVALID" || strings.Contains(checks[1].Actual, "secret") {
		t.Fatalf("credential-bearing server URL accepted/leaked: %+v", checks)
	}

	insecure := writeKubeconfig(t, dir, "c3", "https://127.0.0.1:"+port, "", true)
	checks = ProbeExistingKubeconfig(context.Background(), insecure)
	if checks[2].ReasonCode != "API_CA_UNAVAILABLE" || !strings.Contains(checks[2].Actual, "insecure") {
		t.Fatalf("insecure-skip-tls-verify not rejected: %+v", checks)
	}

	noCA := writeKubeconfig(t, dir, "c4", "https://127.0.0.1:"+port, "", false)
	checks = ProbeExistingKubeconfig(context.Background(), noCA)
	if checks[2].ReasonCode != "API_CA_UNAVAILABLE" {
		t.Fatalf("missing CA not reported: %+v", checks)
	}
}

func TestKubeconfigProbeTLSEndToEnd(t *testing.T) {
	srv, caData := kubeconfigTLSFixture(t)
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "https://"))
	dir := t.TempDir()

	path := writeKubeconfig(t, dir, "c0", "https://127.0.0.1:"+port, caData, false)
	checks := ProbeExistingKubeconfig(context.Background(), path)
	if len(checks) != 4 || checks[3].Status != StatusPass || checks[3].ReasonCode != "CHECK_PASSED" {
		t.Fatalf("end-to-end TLS did not pass: %+v", checks)
	}
}

func TestKubeconfigProbeWrongCAAndUnreachableServer(t *testing.T) {
	srvA, caA := kubeconfigTLSFixture(t)
	srvB, caB := kubeconfigTLSFixture(t)
	_, portA, _ := net.SplitHostPort(strings.TrimPrefix(srvA.URL, "https://"))
	_, portB, _ := net.SplitHostPort(strings.TrimPrefix(srvB.URL, "https://"))
	dir := t.TempDir()

	// Certificate from server B, but kubeconfig points at server A -> wrong CA.
	wrongCA := writeKubeconfig(t, dir, "c0", "https://127.0.0.1:"+portA, caB, false)
	checks := ProbeExistingKubeconfig(context.Background(), wrongCA)
	if len(checks) != 4 || checks[3].Status != StatusBlocked || !strings.Contains(checks[3].Actual, "unknown CA") {
		t.Fatalf("wrong CA not rejected: %+v", checks)
	}

	// Reachable CA, but no listener on the port -> connection failure.
	unreachable := writeKubeconfig(t, dir, "c1", "https://127.0.0.1:"+portB+"", caA, false)
	checks = ProbeExistingKubeconfig(context.Background(), unreachable)
	if len(checks) != 4 || checks[3].Status != StatusBlocked {
		t.Fatalf("unreachable server not blocked: %+v", checks)
	}
}

func TestKubeconfigProbeBadYAMLAndNoCurrentContext(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(bad, []byte("apiVersion: [unterminated"), 0o600); err != nil {
		t.Fatal(err)
	}
	checks := ProbeExistingKubeconfig(context.Background(), bad)
	if len(checks) != 1 || !strings.Contains(checks[0].Actual, "YAML") {
		t.Fatalf("bad YAML not reported: %+v", checks)
	}

	noCtx := filepath.Join(dir, "noctx.yaml")
	doc := "apiVersion: v1\nkind: Config\nclusters: []\ncontexts: []\n"
	if err := os.WriteFile(noCtx, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	checks = ProbeExistingKubeconfig(context.Background(), noCtx)
	if len(checks) != 1 || !strings.Contains(checks[0].Actual, "current-context") {
		t.Fatalf("missing current-context not reported: %+v", checks)
	}
}
