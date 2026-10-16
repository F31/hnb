package toolkit

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// kubeconfig model is intentionally minimal: only the fields needed to observe
// the API source path are parsed. No credentials, client certificates or
// execution plugins are accepted here.
type kubeconfigDoc struct {
	CurrentContext string              `yaml:"current-context"`
	Clusters       []kubeconfigCluster `yaml:"clusters"`
	Contexts       []kubeconfigContext `yaml:"contexts"`
}

type kubeconfigCluster struct {
	Name    string               `yaml:"name"`
	Cluster kubeconfigClusterRef `yaml:"cluster"`
}

type kubeconfigClusterRef struct {
	Server                   string `yaml:"server"`
	CertificateAuthority     string `yaml:"certificate-authority"`
	CertificateAuthorityData string `yaml:"certificate-authority-data"`
	InsecureSkipTLSVerify    bool   `yaml:"insecure-skip-tls-verify"`
}

type kubeconfigContext struct {
	Name    string               `yaml:"name"`
	Context kubeconfigContextRef `yaml:"context"`
}

type kubeconfigContextRef struct {
	Cluster string `yaml:"cluster"`
}

// ReadKubeconfigEndpoint resolves the current-context API server as host:port
// without contacting the cluster or reading credentials. It is used for draft
// generation (init --from-kubeconfig); it does not certify the cluster.
func ReadKubeconfigEndpoint(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("kubeconfig unreadable")
	}
	if len(data) > 1<<20 {
		return "", fmt.Errorf("kubeconfig exceeds 1 MiB")
	}
	var doc kubeconfigDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return "", fmt.Errorf("kubeconfig is not valid YAML")
	}
	if doc.CurrentContext == "" {
		return "", fmt.Errorf("current-context is empty")
	}
	currentCluster := ""
	for _, c := range doc.Contexts {
		if c.Name == doc.CurrentContext {
			currentCluster = c.Context.Cluster
			break
		}
	}
	if currentCluster == "" {
		return "", fmt.Errorf("current context does not name a cluster")
	}
	for _, c := range doc.Clusters {
		if c.Name != currentCluster {
			continue
		}
		u, err := url.Parse(c.Cluster.Server)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return "", fmt.Errorf("current cluster server must be an https URL without credentials or query")
		}
		host := u.Hostname()
		port := u.Port()
		if port == "" {
			port = "443"
		}
		return net.JoinHostPort(host, port), nil
	}
	return "", fmt.Errorf("current cluster not found in clusters")
}

// ProbeExistingKubeconfig observes the installer -> existing API source path
// from the declared kubeconfig: file integrity, https-only server reference,
// CA identity (embedded data or file) and a live TLS handshake. It never reads
// credentials, executes plugins or modifies the file.
func ProbeExistingKubeconfig(ctx context.Context, kubeconfigPath string) []PreflightCheck {
	pathCheck := observedCheck("existing.kubeconfig-file", "cluster", "readable, parseable kubeconfig with a current context", "not observed", "KUBECONFIG_UNAVAILABLE", "Provide the operator's kubeconfig for the existing cluster; credentials are never read.", false)
	data, err := os.ReadFile(kubeconfigPath)
	if err != nil {
		pathCheck.Actual = "kubeconfig unreadable"
		return []PreflightCheck{pathCheck}
	}
	if len(data) > 1<<20 {
		pathCheck.Actual = "kubeconfig exceeds 1 MiB"
		return []PreflightCheck{pathCheck}
	}
	var doc kubeconfigDoc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		pathCheck.Actual = "kubeconfig is not valid YAML"
		return []PreflightCheck{pathCheck}
	}
	if doc.CurrentContext == "" {
		pathCheck.Actual = "current-context is empty"
		return []PreflightCheck{pathCheck}
	}
	var currentCluster string
	foundContext := false
	for _, c := range doc.Contexts {
		if c.Name == doc.CurrentContext {
			currentCluster = c.Context.Cluster
			foundContext = true
			break
		}
	}
	if !foundContext || currentCluster == "" {
		pathCheck.Actual = "current context does not name a cluster"
		return []PreflightCheck{pathCheck}
	}
	var cluster *kubeconfigClusterRef
	for i := range doc.Clusters {
		if doc.Clusters[i].Name == currentCluster {
			cluster = &doc.Clusters[i].Cluster
			break
		}
	}
	if cluster == nil {
		pathCheck.Actual = "current cluster not found in clusters"
		return []PreflightCheck{pathCheck}
	}
	pathCheck.Status, pathCheck.ReasonCode, pathCheck.Actual, pathCheck.Remediation = StatusPass, "CHECK_PASSED", "kubeconfig parses; current context resolves a cluster", ""
	checks := []PreflightCheck{pathCheck}

	u, err := url.Parse(cluster.Server)
	serverCheck := observedCheck("existing.api-server", "cluster", "https API server for the current cluster", "not observed", "API_SERVER_REFERENCE_INVALID", "Declare an https:// API server host:port; credentials/query strings are not accepted.", false)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		serverCheck.Actual = "server must be an https URL without credentials or query"
		checks = append(checks, serverCheck)
		return checks
	}
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = "443"
	}
	if net.ParseIP(host) == nil && strings.ContainsAny(host, "\r\n\x00/\\") {
		serverCheck.Actual = "server hostname contains invalid characters"
		checks = append(checks, serverCheck)
		return checks
	}
	serverCheck.Status, serverCheck.ReasonCode = StatusPass, "CHECK_PASSED"
	serverCheck.Actual = fmt.Sprintf("current cluster API server https://%s:%s declared", host, port)
	checks = append(checks, serverCheck)

	// CA identity: embedded data wins; otherwise the referenced CA file must exist.
	var caPEM []byte
	caCheck := observedCheck("existing.api-ca", "cluster", "CA identity for the API server is present and parseable", "not observed", "API_CA_UNAVAILABLE", "Provide certificate-authority-data or a readable certificate-authority file.", false)
	if cluster.InsecureSkipTLSVerify {
		caCheck.Actual = "insecure-skip-tls-verify is set; TLS identity is not verified"
		checks = append(checks, caCheck)
		return checks
	}
	switch {
	case cluster.CertificateAuthorityData != "":
		raw, decErr := base64.StdEncoding.DecodeString(cluster.CertificateAuthorityData)
		if decErr != nil {
			caCheck.Actual = "certificate-authority-data is not valid base64"
			checks = append(checks, caCheck)
			return checks
		}
		caPEM = raw
	case cluster.CertificateAuthority != "":
		raw, readErr := os.ReadFile(cluster.CertificateAuthority)
		if readErr != nil {
			caCheck.Actual = "certificate-authority file unreadable"
			checks = append(checks, caCheck)
			return checks
		}
		caPEM = raw
	default:
		caCheck.Actual = "no CA data or file declared; TLS cannot be verified"
		checks = append(checks, caCheck)
		return checks
	}
	pool := x509.NewCertPool()
	block, rest := pem.Decode(caPEM)
	for block != nil {
		if block.Type == "CERTIFICATE" {
			if cert, parseErr := x509.ParseCertificate(block.Bytes); parseErr == nil {
				pool.AddCert(cert)
			}
		}
		block, rest = pem.Decode(rest)
	}
	if len(pool.Subjects()) == 0 {
		caCheck.Actual = "CA data contains no parseable certificates"
		checks = append(checks, caCheck)
		return checks
	}
	caCheck.Status, caCheck.ReasonCode = StatusPass, "CHECK_PASSED"
	caCheck.Actual = "API CA certificates parsed into a verification pool"
	checks = append(checks, caCheck)

	// Live handshake with the parsed CA; the server's chain must verify and the
	// leaf must be valid now. Hostname verification uses the declared host.
	tlsCheck := observedCheck("existing.api-tls", "cluster", "installer -> API server TLS handshake verifies against the declared CA", "not observed", "API_TLS_HANDSHAKE_FAILED", "Check that the kubeconfig CA matches the current API server certificate.", false)
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	conn, dialErr := tls.DialWithDialer(dialer, "tcp", net.JoinHostPort(host, port), &tls.Config{
		RootCAs:            pool,
		ServerName:         host,
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: false,
	})
	if dialErr != nil {
		var unknownCA x509.UnknownAuthorityError
		var hostnameErr x509.HostnameError
		var certInvalid x509.CertificateInvalidError
		switch {
		case errors.As(dialErr, &unknownCA):
			tlsCheck.Actual = "TLS verification failed: server certificate signed by an unknown CA"
		case errors.As(dialErr, &hostnameErr):
			tlsCheck.Actual = "TLS verification failed: certificate SAN does not match the API host"
		case errors.As(dialErr, &certInvalid):
			tlsCheck.Actual = "TLS verification failed: certificate is expired or otherwise invalid"
		default:
			tlsCheck.Actual = "TLS handshake failed: " + sanitizeProbeError(dialErr)
		}
		checks = append(checks, tlsCheck)
		return checks
	}
	conn.Close()
	tlsCheck.Status, tlsCheck.ReasonCode = StatusPass, "CHECK_PASSED"
	tlsCheck.Actual = "installer -> API server TLS handshake verified against the declared CA"
	checks = append(checks, tlsCheck)
	return checks
}

// sanitizeProbeError removes newlines to prevent log/error injection from a
// remote server; URL/credential fragments are never echoed.
func sanitizeProbeError(err error) string {
	msg := err.Error()
	msg = strings.ReplaceAll(msg, "\n", " ")
	msg = strings.ReplaceAll(msg, "\r", " ")
	if len(msg) > 200 {
		msg = msg[:200]
	}
	return msg
}
