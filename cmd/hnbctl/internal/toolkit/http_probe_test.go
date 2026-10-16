package toolkit

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func bodyDigest(data string) string { return fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(data))) }

func TestContentMetadataDoesNotProveDownload(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(200)
			return
		}
		w.WriteHeader(503)
		_, _ = io.WriteString(w, "password=secret")
	}))
	defer server.Close()
	checks := NewHTTPProbe(server.Client()).ProbeContent(context.Background(), "fixture", server.URL, bodyDigest("fixture"), 1024)
	if len(checks) != 2 || checks[0].Status != StatusPass || checks[1].ReasonCode != "ARTIFACT_CONTENT_UNAVAILABLE" || strings.Contains(checks[1].Actual, "secret") {
		t.Fatalf("HEAD success concealed content failure: %+v", checks)
	}
}

func TestContentIdentityCompletenessAndBudget(t *testing.T) {
	for _, tc := range []struct {
		name, body, digest, length, reason string
		budget                             int64
	}{
		{"valid", "fixture", bodyDigest("fixture"), "", "CHECK_PASSED", 1024},
		{"mismatch", "fixture", bodyDigest("different"), "", "ARTIFACT_DIGEST_MISMATCH", 1024},
		{"oversized", "fixture", bodyDigest("fixture"), "", "ARTIFACT_PROBE_LIMIT_EXCEEDED", 3},
		{"truncated", "fixture", bodyDigest("fixture"), "30", "ARTIFACT_CONTENT_INCOMPLETE", 1024},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodHead {
					return
				}
				if tc.length != "" {
					w.Header().Set("Content-Length", tc.length)
				}
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			checks := NewHTTPProbe(server.Client()).ProbeContent(context.Background(), "fixture", server.URL, tc.digest, tc.budget)
			if len(checks) != 2 || checks[1].ReasonCode != tc.reason {
				t.Fatalf("wrong content verdict: %+v", checks)
			}
		})
	}
}

func TestProbeNeverFollowsRedirectsOrRequestsCredentialURLs(t *testing.T) {
	var destinationCalls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/destination" {
			destinationCalls.Add(1)
			return
		}
		http.Redirect(w, r, "/destination", http.StatusFound)
	}))
	defer server.Close()
	probe := NewHTTPProbe(server.Client())
	checks := probe.ProbeContent(context.Background(), "fixture", server.URL, bodyDigest("fixture"), 1024)
	if destinationCalls.Load() != 0 || checks[1].Status != StatusBlocked {
		t.Fatal("redirect was silently followed")
	}
	for _, address := range []string{"http://example.invalid", "https://user:secret@example.invalid/file", server.URL + "?token=secret"} {
		checks := probe.ProbeContent(context.Background(), "fixture", address, bodyDigest("fixture"), 1024)
		if len(checks) != 1 || checks[0].ReasonCode != "ARTIFACT_REFERENCE_INVALID" || strings.Contains(checks[0].Actual, "secret") {
			t.Fatal("credential-bearing reference accepted/leaked")
		}
	}
}

func tlsFixture(t *testing.T, expired, wrongSAN bool) (*httptest.Server, *http.Client) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "fixture"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
	if expired {
		template.NotBefore, template.NotAfter = time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour)
	}
	if wrongSAN {
		template.IPAddresses = nil
		template.DNSNames = []string{"different.invalid"}
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
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "fixture") }))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.TLS = &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}, MinVersion: tls.VersionTLS12}
	server.StartTLS()
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}}
	t.Cleanup(func() { client.CloseIdleConnections(); server.Close() })
	return server, client
}

func TestEntryTLSChainSANAndValidity(t *testing.T) {
	for _, tc := range []struct {
		name                         string
		expired, wrongSAN, unknownCA bool
		reason                       string
	}{
		{"valid", false, false, false, "CHECK_PASSED"},
		{"expired", true, false, false, "TLS_VERIFICATION_FAILED"},
		{"wrong-san", false, true, false, "TLS_VERIFICATION_FAILED"},
		{"unknown-ca", false, false, true, "TLS_VERIFICATION_FAILED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, client := tlsFixture(t, tc.expired, tc.wrongSAN)
			if tc.unknownCA {
				client = nil
			}
			check := NewHTTPProbe(client).ProbeEntry(context.Background(), server.URL)
			if check.ReasonCode != tc.reason {
				t.Fatalf("wrong TLS verdict: %+v", check)
			}
		})
	}
}
