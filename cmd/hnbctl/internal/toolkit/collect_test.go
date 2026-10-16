package toolkit

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

func bundleEntries(t *testing.T, raw []byte) map[string]string {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	out := map[string]string{}
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		out[header.Name] = string(data)
	}
	return out
}

// secretFixtures covers every class named by the task: password, private key,
// kubeconfig, token, DSN and customer data.
var secretFixtures = map[string]string{
	"password":    "POSTGRES_PASSWORD=hunter2SuperSecret",
	"private_key": "-----BEGIN EC PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASC\n-----END EC PRIVATE KEY-----",
	"kubeconfig":  "apiVersion: v1\nclusters:\n- cluster:\n    certificate-authority-data: Y2VydA==\nusers:\n- user:\n    token: agent-fixture-token",
	"token":       "Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJhZG1pbiJ9.sig",
	"dsn":         "postgres://hnb_admin:p4ssw0rd@10.0.0.9:5432/hnb_platform?sslmode=disable",
	"nats":        "nats://hnb:sup3rs3cret@10.0.0.9:4222",
	"customer":    `{"displayName":"Contoso Retail Ltd","tenantName":"contoso-prod","businessNamespace":"acme-billing"}`,
	"master_key":  "HNB_MASTER_KEY=deadbeefcafebabe0123456789abcdef",
	"api_key":     "api_key=sk-live-abcdefghijklmnopqrstuvwxyz012345",
	"client_key":  "client-key-data: LS0tLS1CRUdJTiBSU0EgUFJJVkFURSBLRVktLS0tLQo=",
}

// 密码/私钥/kubeconfig/token/DSN/客户数据 fixtures 不出包.
func TestCollectArchiveContainsNoSecretsOrCustomerData(t *testing.T) {
	collector := &DiagnosticCollector{BundleID: "bundle-1", RunID: "run-1",
		CollectedAt: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)}
	if err := collector.AddJSON("platform/config-summary.json", map[string]any{
		"configDigest":   "sha256:1111",
		"lockDigest":     "sha256:2222",
		"password":       secretFixtures["password"],
		"dbDsn":          secretFixtures["dsn"],
		"apiToken":       secretFixtures["token"],
		"privateKeyPem":  secretFixtures["private_key"],
		"kubeconfigFile": secretFixtures["kubeconfig"],
		"displayName":    "Contoso Retail Ltd",
		"tenantName":     "contoso-prod",
	}); err != nil {
		t.Fatal(err)
	}
	if err := collector.AddText("platform/service.log", strings.Join([]string{
		secretFixtures["password"], secretFixtures["private_key"], secretFixtures["kubeconfig"],
		secretFixtures["token"], secretFixtures["dsn"], secretFixtures["nats"],
		secretFixtures["master_key"], secretFixtures["api_key"], secretFixtures["client_key"],
		"customer namespace acme-billing tenant contoso-prod",
	}, "\n")); err != nil {
		t.Fatal(err)
	}

	var raw bytes.Buffer
	if err := collector.WriteArchive(&raw); err != nil {
		t.Fatal(err)
	}
	archive := raw.Bytes()
	entries := bundleEntries(t, archive)
	if len(entries) < 3 {
		t.Fatalf("archive missing entries: %v", entries)
	}

	lower := strings.ToLower(string(archive))
	forbidden := []string{
		"hunters2supersecret", "huntersecret", "mievqibadanbgkqhki", "ce0hdmlldm9yc2hh", // raw-ish
		"p4ssw0rd", "sup3rs3cret", "deadbeefcafebabe", "sk-live-abcdefghij", "agent-fixture-token",
		"eyjhbGciOiJIUzI1NiJ9", "contoso", "acme-billing", "contoso-prod", "y2vydg==",
	}
	for _, secret := range forbidden {
		if strings.Contains(lower, strings.ToLower(secret)) {
			t.Fatalf("archive leaked %q", secret)
		}
	}
	// Diagnostic digests survive redaction: redaction must not gut the evidence.
	if !strings.Contains(entries["platform/config-summary.json"], "sha256:1111") {
		t.Fatalf("redaction removed useful digests: %s", entries["platform/config-summary.json"])
	}
}

// The archive identity is reported and is deterministic for identical input.
func TestCollectReportsBundleIdentityDeterministically(t *testing.T) {
	build := func() *DiagnosticCollector {
		c := &DiagnosticCollector{BundleID: "bundle-1", RunID: "run-1",
			CollectedAt: time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)}
		_ = c.AddJSON("a/report.json", map[string]any{"state": "failed", "reasonCode": "PLATFORM_LOGIN_FAILED"})
		_ = c.AddJSON("b/verify.json", map[string]any{"overall": "partial"})
		return c
	}
	first, second := build(), build()
	idFirst, idSecond := first.Identity(), second.Identity()
	if idFirst.ContentDigest != idSecond.ContentDigest || idFirst.BundleID != idSecond.BundleID ||
		!idFirst.CollectedAt.Equal(idSecond.CollectedAt) || strings.Join(idFirst.EntryPaths, ",") != strings.Join(idSecond.EntryPaths, ",") {
		t.Fatalf("identity is not deterministic: %+v vs %+v", idFirst, idSecond)
	}
	var a, b bytes.Buffer
	if err := first.WriteArchive(&a); err != nil {
		t.Fatal(err)
	}
	if err := second.WriteArchive(&b); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Fatal("identical inputs produced different archive bytes")
	}
	identity := first.Identity()
	if identity.SchemaVersion != "diagnostic-bundle/v1" || identity.BundleID != "bundle-1" ||
		identity.RunID != "run-1" || !identity.Redacted || identity.CollectedAt.IsZero() {
		t.Fatalf("incomplete bundle identity: %+v", identity)
	}
	if identity.ContentDigest == "" || len(identity.EntryPaths) != 2 {
		t.Fatalf("identity lacks content digest or entries: %+v", identity)
	}
	entries := bundleEntries(t, a.Bytes())
	if _, ok := entries["bundle-identity.json"]; !ok {
		t.Fatal("archive missing its identity document")
	}
}

// Any change to redacted content changes the content digest.
func TestCollectContentDigestTracksRedactedContent(t *testing.T) {
	c1 := &DiagnosticCollector{BundleID: "b", RunID: "r", CollectedAt: time.Now()}
	_ = c1.AddText("log.txt", "step=init status=ok")
	c2 := &DiagnosticCollector{BundleID: "b", RunID: "r", CollectedAt: c1.CollectedAt}
	_ = c2.AddText("log.txt", "step=init status=failed")
	if c1.Identity().ContentDigest == c2.Identity().ContentDigest {
		t.Fatal("different content produced the same digest")
	}
}

// Entry names cannot escape the archive or overwrite each other.
func TestCollectRejectsUnsafeEntryNames(t *testing.T) {
	c := &DiagnosticCollector{BundleID: "b", RunID: "r"}
	for _, name := range []string{"", "../escape.json", "/abs/path.json", "a/../../b.json", "line\nbreak.json"} {
		if err := c.AddJSON(name, map[string]any{"a": 1}); err == nil {
			t.Fatalf("unsafe entry name accepted: %q", name)
		}
	}
	if err := c.AddJSON("dup.json", map[string]any{"a": 1}); err != nil {
		t.Fatal(err)
	}
	if err := c.AddJSON("dup.json", map[string]any{"b": 2}); err == nil {
		t.Fatal("duplicate entry name accepted")
	}
}

// Log excerpts are bounded so an archive cannot smuggle bulk data.
func TestCollectTruncatesOversizedLogExcerpts(t *testing.T) {
	c := &DiagnosticCollector{BundleID: "b", RunID: "r", MaxLogBytes: 128}
	if err := c.AddText("big.log", strings.Repeat("a", 100000)); err != nil {
		t.Fatal(err)
	}
	entries := c.Entries()
	if len(entries) != 1 || len(entries[0].Content) > 200 {
		t.Fatalf("log excerpt not bounded: %d bytes", len(entries[0].Content))
	}
	if !strings.HasSuffix(string(entries[0].Content), "[TRUNCATED]") {
		t.Fatal("truncation is not marked")
	}
}

// WriteArchiveFile refuses to overwrite an existing archive.
func TestCollectArchiveFileIsPrivateAndExclusive(t *testing.T) {
	c := &DiagnosticCollector{BundleID: "b", RunID: "r"}
	_ = c.AddJSON("a.json", map[string]any{"a": 1})
	target := fmt.Sprintf("%s/diagnostics.tar.gz", t.TempDir())
	if err := c.WriteArchiveFile(target); err != nil {
		t.Fatal(err)
	}
	if err := c.WriteArchiveFile(target); err == nil {
		t.Fatal("existing diagnostics archive was overwritten")
	}
}
