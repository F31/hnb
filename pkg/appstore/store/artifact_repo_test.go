package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/F31/hnb/pkg/appstore"
)

type scriptedResult struct {
	columns []string
	values  []driver.Value
}

type scriptedConnector struct {
	queries   []scriptedResult
	queryText []string
	execs     []string
	execArgs  [][]driver.NamedValue
	committed bool
}

func (c *scriptedConnector) Connect(context.Context) (driver.Conn, error) {
	return &scriptedConn{connector: c}, nil
}
func (c *scriptedConnector) Driver() driver.Driver { return captureDriver{} }

type scriptedConn struct{ connector *scriptedConnector }

func (c *scriptedConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare is unsupported")
}
func (c *scriptedConn) Close() error { return nil }
func (c *scriptedConn) Begin() (driver.Tx, error) {
	return &scriptedTx{connector: c.connector}, nil
}
func (c *scriptedConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return c.Begin()
}
func (c *scriptedConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.connector.execs = append(c.connector.execs, query)
	c.connector.execArgs = append(c.connector.execArgs, append([]driver.NamedValue(nil), args...))
	return driver.RowsAffected(1), nil
}
func (c *scriptedConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	c.connector.queryText = append(c.connector.queryText, query)
	if len(c.connector.queries) == 0 {
		return nil, errors.New("unexpected query")
	}
	result := c.connector.queries[0]
	c.connector.queries = c.connector.queries[1:]
	return &scriptedRows{columns: result.columns, values: result.values}, nil
}

type scriptedTx struct{ connector *scriptedConnector }

func (t *scriptedTx) Commit() error {
	t.connector.committed = true
	return nil
}
func (t *scriptedTx) Rollback() error { return nil }

type scriptedRows struct {
	columns []string
	values  []driver.Value
	read    bool
}

func (r *scriptedRows) Columns() []string { return r.columns }
func (r *scriptedRows) Close() error      { return nil }
func (r *scriptedRows) Next(dest []driver.Value) error {
	if r.read {
		return io.EOF
	}
	copy(dest, r.values)
	r.read = true
	return nil
}

func TestArtifactRepoConfirmUploadCommitsDescriptorAndSession(t *testing.T) {
	now := time.Now()
	connector := &scriptedConnector{queries: []scriptedResult{
		{columns: []string{"status", "artifact_id", "release_id", "expires_at"}, values: []driver.Value{"pending", nil, nil, now.Add(time.Hour)}},
		artifactScriptedResult(now),
	}}
	db := sql.OpenDB(connector)
	defer db.Close()

	artifact, err := NewArtifactRepo(db).ConfirmUpload("session-a", "tenant-a", &appstore.ArtifactDescriptor{
		ID: "artifact-a", TenantID: "tenant-a", Name: "artifact.bin", ArtifactType: appstore.ArtGeneric,
		Repository: "hnb/generic", Digest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		VerificationStatus: appstore.ArtifactVerified, LifecycleState: appstore.ArtifactActive,
	})
	if err != nil {
		t.Fatalf("ConfirmUpload failed: %v", err)
	}
	if artifact.ID != "artifact-a" || !connector.committed {
		t.Fatalf("unexpected result artifact=%+v committed=%v", artifact, connector.committed)
	}
	if len(connector.queries) != 0 {
		t.Fatalf("unused scripted queries: %d", len(connector.queries))
	}
}

func TestArtifactRepoConfirmUploadRejectsExpiredSession(t *testing.T) {
	connector := &scriptedConnector{queries: []scriptedResult{{
		columns: []string{"status", "artifact_id", "release_id", "expires_at"},
		values:  []driver.Value{"pending", nil, nil, time.Now().Add(-time.Minute)},
	}}}
	db := sql.OpenDB(connector)
	defer db.Close()

	_, err := NewArtifactRepo(db).ConfirmUpload("session-a", "tenant-a", &appstore.ArtifactDescriptor{})
	if !errors.Is(err, ErrUploadSessionExpired) {
		t.Fatalf("expected ErrUploadSessionExpired, got %v", err)
	}
	if connector.committed {
		t.Fatal("expired confirmation committed")
	}
}

func TestArtifactRepoConfirmUploadIsIdempotentForCompletedSession(t *testing.T) {
	now := time.Now()
	digest := "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	connector := &scriptedConnector{queries: []scriptedResult{
		{columns: []string{"status", "artifact_id", "release_id", "expires_at"}, values: []driver.Value{"completed", "artifact-a", nil, now.Add(time.Hour)}},
		{columns: []string{"id", "tenant_id", "package_id", "storage_profile_id", "name", "artifact_type", "media_type", "repository", "registry_url", "digest", "size_bytes", "verification_status", "lifecycle_state", "metadata", "created_at", "updated_at"}, values: []driver.Value{
			"artifact-a", "tenant-a", nil, nil, "artifact.bin", "generic", nil, "hnb/generic", nil, digest, int64(42), "verified", "active", []byte(`{}`), now, now,
		}},
	}}
	db := sql.OpenDB(connector)
	defer db.Close()

	artifact, err := NewArtifactRepo(db).ConfirmUpload("session-a", "tenant-a", &appstore.ArtifactDescriptor{})
	if err != nil {
		t.Fatalf("ConfirmUpload failed: %v", err)
	}
	if artifact.ID != "artifact-a" || artifact.Digest != digest {
		t.Fatalf("unexpected artifact: %+v", artifact)
	}
	if !connector.committed {
		t.Fatal("idempotent confirmation did not commit")
	}
}

func TestArtifactRepoConfirmUploadLinksDraftReleaseAtomically(t *testing.T) {
	now := time.Now()
	connector := &scriptedConnector{queries: []scriptedResult{
		{columns: []string{"status", "artifact_id", "release_id", "expires_at"}, values: []driver.Value{"uploading", nil, "release-a", now.Add(time.Hour)}},
		{columns: []string{"status", "manifest"}, values: []driver.Value{"draft", []byte(`{"kind":"service","artifacts":[]}`)}},
		artifactScriptedResult(now),
	}}
	db := sql.OpenDB(connector)
	defer db.Close()

	_, err := NewArtifactRepo(db).ConfirmUpload("session-a", "tenant-a", &appstore.ArtifactDescriptor{
		ID: "artifact-a", TenantID: "tenant-a", Name: "artifact.bin", ArtifactType: appstore.ArtGeneric,
		Repository: "hnb/generic", Digest: "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		VerificationStatus: appstore.ArtifactVerified, LifecycleState: appstore.ArtifactActive,
	})
	if err != nil {
		t.Fatalf("ConfirmUpload failed: %v", err)
	}
	if !connector.committed || len(connector.execs) != 3 {
		t.Fatalf("expected manifest, relation, and session writes in committed transaction: %+v", connector.execs)
	}
	if !strings.Contains(connector.execs[0], "UPDATE releases SET manifest") {
		t.Fatalf("release manifest not synchronized: %s", connector.execs[0])
	}
	manifestJSON, ok := connector.execArgs[0][1].Value.([]byte)
	if !ok {
		t.Fatalf("manifest argument is %T", connector.execArgs[0][1].Value)
	}
	var manifest struct {
		Kind      string               `json:"kind"`
		Artifacts []releaseArtifactRef `json:"artifacts"`
	}
	if err := json.Unmarshal(manifestJSON, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Kind != "service" || len(manifest.Artifacts) != 1 || manifest.Artifacts[0].Name != "artifact.bin" || manifest.Artifacts[0].Purpose != "runtime" {
		t.Fatalf("unexpected synchronized manifest: %s", manifestJSON)
	}
	_, expectedDigest, err := appstore.EncodeReleaseManifest(map[string]any{
		"kind": "service",
		"artifacts": []any{map[string]any{
			"name": "artifact.bin", "digest": manifest.Artifacts[0].Digest, "purpose": "runtime",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if connector.execArgs[0][2].Value != expectedDigest {
		t.Fatalf("manifest digest = %v, want %s", connector.execArgs[0][2].Value, expectedDigest)
	}
	if !strings.Contains(connector.execs[1], "INSERT INTO release_artifacts") || !strings.Contains(connector.execs[1], "'runtime'") || !strings.Contains(connector.execs[1], "ON CONFLICT") {
		t.Fatalf("unsafe release artifact insert: %s", connector.execs[1])
	}
}

func TestArtifactRepoConfirmUploadRejectsPublishedRelease(t *testing.T) {
	now := time.Now()
	connector := &scriptedConnector{queries: []scriptedResult{
		{columns: []string{"status", "artifact_id", "release_id", "expires_at"}, values: []driver.Value{"pending", nil, "release-a", now.Add(time.Hour)}},
		{columns: []string{"status", "manifest"}, values: []driver.Value{"published", []byte(`{}`)}},
	}}
	db := sql.OpenDB(connector)
	defer db.Close()

	_, err := NewArtifactRepo(db).ConfirmUpload("session-a", "tenant-a", &appstore.ArtifactDescriptor{})
	if !errors.Is(err, ErrUploadReleaseState) {
		t.Fatalf("expected ErrUploadReleaseState, got %v", err)
	}
	if connector.committed || len(connector.execs) != 0 {
		t.Fatal("published release confirmation performed writes")
	}
}

func TestAppendReleaseManifestArtifactDoesNotDuplicateDigest(t *testing.T) {
	digest := "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	data, _, err := appendReleaseManifestArtifact([]byte(`{"kind":"service","generation":9007199254740993,"artifacts":[{"name":"existing","digest":"`+digest+`","purpose":"runtime","extra":true}]}`), "new", digest)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	artifacts := manifest["artifacts"].([]any)
	if manifest["kind"] != "service" || len(artifacts) != 1 || artifacts[0].(map[string]any)["extra"] != true || !strings.Contains(string(data), `"generation":9007199254740993`) {
		t.Fatalf("manifest fields were not preserved: %s", data)
	}
}

func TestArtifactRepoAttachToReleaseRebuildsManifest(t *testing.T) {
	now := time.Now()
	digest := "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	connector := &scriptedConnector{queries: []scriptedResult{
		{columns: []string{"status", "manifest"}, values: []driver.Value{"draft", []byte(`{"kind":"service","artifacts":[]}`)}},
		artifactScriptedResult(now),
		{columns: []string{"name", "digest", "purpose"}, values: []driver.Value{"artifact.bin", digest, "runtime"}},
	}}
	db := sql.OpenDB(connector)
	defer db.Close()

	artifact, err := NewArtifactRepo(db).AttachToRelease("release-a", "artifact-a", "tenant-a")
	if err != nil {
		t.Fatal(err)
	}
	if artifact.ID != "artifact-a" || !connector.committed || len(connector.execs) != 2 {
		t.Fatalf("attach did not commit atomically: artifact=%+v execs=%+v", artifact, connector.execs)
	}
	if !strings.Contains(connector.execs[0], "INSERT INTO release_artifacts") || !strings.Contains(connector.execs[0], "ON CONFLICT") {
		t.Fatalf("attach relation write is not idempotent: %s", connector.execs[0])
	}
	assertRebuiltManifest(t, connector.execArgs[1], "service", "artifact.bin", digest)
	if !strings.Contains(connector.queryText[0], "pub.tenant_id=$2") || !strings.Contains(connector.queryText[1], "tenant_id=$2") {
		t.Fatalf("attach queries are not tenant scoped: %+v", connector.queryText)
	}
}

func TestArtifactRepoDetachFromReleaseRebuildsManifest(t *testing.T) {
	now := time.Now()
	digest := "sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	connector := &scriptedConnector{queries: []scriptedResult{
		{columns: []string{"status", "manifest"}, values: []driver.Value{"draft", []byte(`{"kind":"service","artifacts":[{"name":"removed","digest":"sha256:removed","purpose":"runtime"}]}`)}},
		artifactScriptedResult(now),
		{columns: []string{"name", "digest", "purpose"}, values: []driver.Value{"remaining.bin", digest, "runtime"}},
	}}
	db := sql.OpenDB(connector)
	defer db.Close()

	if err := NewArtifactRepo(db).DetachFromRelease("release-a", "artifact-a", "tenant-a"); err != nil {
		t.Fatal(err)
	}
	if !connector.committed || len(connector.execs) != 2 || !strings.Contains(connector.execs[0], "DELETE FROM release_artifacts") {
		t.Fatalf("detach did not commit atomically: %+v", connector.execs)
	}
	assertRebuiltManifest(t, connector.execArgs[1], "service", "remaining.bin", digest)
}

func TestArtifactRepoAttachToReleaseRejectsNonDraft(t *testing.T) {
	connector := &scriptedConnector{queries: []scriptedResult{{
		columns: []string{"status", "manifest"}, values: []driver.Value{"published", []byte(`{}`)},
	}}}
	db := sql.OpenDB(connector)
	defer db.Close()

	_, err := NewArtifactRepo(db).AttachToRelease("release-a", "artifact-a", "tenant-a")
	if !errors.Is(err, ErrUploadReleaseState) || connector.committed || len(connector.execs) != 0 {
		t.Fatalf("non-draft attach result: err=%v committed=%v", err, connector.committed)
	}
}

func assertRebuiltManifest(t *testing.T, args []driver.NamedValue, kind, name, digest string) {
	t.Helper()
	manifestJSON, ok := args[1].Value.([]byte)
	if !ok {
		t.Fatalf("manifest argument is %T", args[1].Value)
	}
	var manifest struct {
		Kind      string               `json:"kind"`
		Artifacts []releaseArtifactRef `json:"artifacts"`
	}
	if err := json.Unmarshal(manifestJSON, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Kind != kind || len(manifest.Artifacts) != 1 || manifest.Artifacts[0].Name != name || manifest.Artifacts[0].Digest != digest || manifest.Artifacts[0].Purpose != "runtime" {
		t.Fatalf("unexpected rebuilt manifest: %s", manifestJSON)
	}
	_, expectedDigest, err := appstore.EncodeReleaseManifest(map[string]any{
		"kind": kind,
		"artifacts": []any{map[string]any{
			"name": name, "digest": digest, "purpose": "runtime",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if args[2].Value != expectedDigest {
		t.Fatalf("manifest digest = %v, want %s", args[2].Value, expectedDigest)
	}
}

func artifactScriptedResult(now time.Time) scriptedResult {
	return scriptedResult{
		columns: []string{"id", "tenant_id", "package_id", "storage_profile_id", "name", "artifact_type", "media_type", "repository", "registry_url", "digest", "size_bytes", "verification_status", "lifecycle_state", "metadata", "created_at", "updated_at"},
		values: []driver.Value{
			"artifact-a", "tenant-a", nil, nil, "artifact.bin", "generic", nil, "hnb/generic", nil,
			"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", int64(42), "verified", "active", []byte(`{}`), now, now,
		},
	}
}
