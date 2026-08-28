package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/F31/hnb/pkg/appstore"
)

type captureConnector struct {
	query    string
	args     []driver.NamedValue
	affected int64
}

func (c *captureConnector) Connect(context.Context) (driver.Conn, error) {
	return &captureConn{capture: c}, nil
}
func (c *captureConnector) Driver() driver.Driver { return captureDriver{} }

type captureDriver struct{}

func (captureDriver) Open(string) (driver.Conn, error) { return nil, errors.New("use connector") }

type captureConn struct{ capture *captureConnector }

func (c *captureConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare is unsupported")
}
func (c *captureConn) Close() error { return nil }
func (c *captureConn) Begin() (driver.Tx, error) {
	return nil, errors.New("transactions are unsupported")
}
func (c *captureConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.capture.query, c.capture.args = query, append([]driver.NamedValue(nil), args...)
	return driver.RowsAffected(c.capture.affected), nil
}
func (c *captureConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.capture.query, c.capture.args = query, append([]driver.NamedValue(nil), args...)
	return emptyRows{}, nil
}

type emptyRows struct{}

func (emptyRows) Columns() []string         { return []string{"id"} }
func (emptyRows) Close() error              { return nil }
func (emptyRows) Next([]driver.Value) error { return io.EOF }

func captureDB() (*sql.DB, *captureConnector) {
	connector := &captureConnector{}
	return sql.OpenDB(connector), connector
}

func TestRepositoryReadsCarryPublisherTenantPredicate(t *testing.T) {
	db, capture := captureDB()
	defer db.Close()

	if _, err := NewPublisherRepo(db).Get("publisher-a", "tenant-a"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("publisher get error = %v", err)
	}
	assertTenantSQL(t, capture, "publishers WHERE id=$1 AND tenant_id=$2")

	if _, err := NewProductRepo(db).Get("product-a", "tenant-a"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("product get error = %v", err)
	}
	assertTenantSQL(t, capture, "JOIN publishers")

	if _, err := NewReleaseRepo(db).Get("release-a", "tenant-a"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("release get error = %v", err)
	}
	assertTenantSQL(t, capture, "JOIN publishers")

	if _, err := NewApplicationRepo(db).Get("application-a", "tenant-a"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("application get error = %v", err)
	}
	assertTenantSQL(t, capture, "tenant_id=$2")
}

func TestRepositoryWritesReturnNotFoundForForeignAssociations(t *testing.T) {
	db, capture := captureDB()
	defer db.Close()

	product := &appstore.Product{ID: "product-a", PublisherID: "publisher-a", Name: "p", DisplayName: "P", Category: appstore.CatApplication, Status: appstore.ProdDraft}
	if err := NewProductRepo(db).Create(product, "tenant-a"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("product create error = %v", err)
	}
	assertTenantSQL(t, capture, "pub.tenant_id=$12")

	release := &appstore.Release{ID: "release-a", ProductID: "product-a", Version: "1", ManifestDigest: "sha256:test", Status: appstore.RelDraft, CreatedBy: "subject"}
	if err := NewReleaseRepo(db).Create(release, "tenant-a"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("release create error = %v", err)
	}
	assertTenantSQL(t, capture, "pub.tenant_id=$10")

	application := &appstore.Application{ID: "application-a", ProductID: "product-a", ReleaseID: "release-a", Name: "app", Status: appstore.AppDeploying}
	if err := NewApplicationRepo(db).Create(application, "tenant-a"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("application create error = %v", err)
	}
	assertTenantSQL(t, capture, "pub.tenant_id=$13")
}

func TestReleaseRepoComputesManifestDigestOnCreateAndUpdate(t *testing.T) {
	manifest := map[string]any{"kind": "service"}
	_, expectedDigest, err := appstore.EncodeReleaseManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}

	db, capture := captureDB()
	capture.affected = 1
	release := &appstore.Release{
		ID: "release-a", ProductID: "product-a", Version: "1", Manifest: manifest,
		ManifestDigest: "client-value", Status: appstore.RelDraft, CreatedBy: "subject",
	}
	if err := NewReleaseRepo(db).Create(release, "tenant-a"); err != nil {
		t.Fatal(err)
	}
	if release.ManifestDigest != expectedDigest || capture.args[5].Value != expectedDigest {
		t.Fatalf("create trusted client digest: model=%s sql=%v", release.ManifestDigest, capture.args[5].Value)
	}
	db.Close()

	connector := &scriptedConnector{}
	db = sql.OpenDB(connector)
	defer db.Close()
	release.ManifestDigest = "another-client-value"
	if err := NewReleaseRepo(db).Update(release, "tenant-a"); err != nil {
		t.Fatal(err)
	}
	if release.ManifestDigest != expectedDigest || connector.execArgs[0][5].Value != expectedDigest {
		t.Fatalf("update trusted client digest: model=%s sql=%v", release.ManifestDigest, connector.execArgs[0][5].Value)
	}
}

func TestPublishAndApplicationMutationAreTenantBounded(t *testing.T) {
	db, capture := captureDB()
	defer db.Close()

	if err := NewReleaseRepo(db).Publish("release-a", "tenant-a"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("publish error = %v", err)
	}
	assertTenantSQL(t, capture, "pub.tenant_id=$3")
	if strings.Contains(capture.query, "updated_at") {
		t.Fatalf("release publish references nonexistent updated_at: %s", capture.query)
	}

	if err := NewApplicationRepo(db).UpdateStatus("application-a", "tenant-a", appstore.AppUninstalling); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("update status error = %v", err)
	}
	assertTenantSQL(t, capture, "tenant_id=$3")
	if err := NewApplicationRepo(db).Delete("application-a", "tenant-a"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("delete error = %v", err)
	}
	assertTenantSQL(t, capture, "tenant_id=$2")
}

func TestReleaseRepoRejectsMalformedArtifactDigest(t *testing.T) {
	db, _ := captureDB()
	defer db.Close()

	release := &appstore.Release{
		ID:             "release-a",
		ProductID:      "product-a",
		Version:        "1",
		ManifestDigest: "sha256:test",
		Status:         appstore.RelDraft,
		CreatedBy:      "subject",
		Manifest:       map[string]any{"artifacts": []map[string]any{{"name": "image", "digest": "repo/app:latest"}}},
	}
	if err := NewReleaseRepo(db).Create(release, "tenant-a"); !errors.Is(err, ErrInvalidArtifactReference) {
		t.Fatalf("expected ErrInvalidArtifactReference, got %v", err)
	}
}

func TestReleaseRepoPublishRequiresNormalizedVerifiedArtifacts(t *testing.T) {
	db, capture := captureDB()
	defer db.Close()

	if err := NewReleaseRepo(db).Publish("release-a", "tenant-a"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("publish error = %v", err)
	}
	for _, fragment := range []string{"AND EXISTS (", "SELECT 1 FROM release_artifacts ra WHERE ra.release_id=r.id", "jsonb_array_length", "verification_status <> 'verified'", "lifecycle_state <> 'active'"} {
		if !strings.Contains(capture.query, fragment) {
			t.Fatalf("publish query missing %q: %s", fragment, capture.query)
		}
	}
}

func TestUploadSessionRepoCreateIncludesTenant(t *testing.T) {
	db, capture := captureDB()
	defer db.Close()

	releaseID := "release-a"
	session := &appstore.UploadSession{
		ID:           "session-a",
		TenantID:     "tenant-a",
		ReleaseID:    &releaseID,
		Filename:     "test.jar",
		ArtifactType: "oci_image",
		SizeBytes:    1024,
		Status:       appstore.SessionPending,
		HarborURL:    "https://harbor.example.com",
		Repository:   "hnb/jars",
		RobotID:      1,
		RobotName:    "robot$upload-test",
		ExpiresAt:    time.Now().Add(3600 * time.Second),
	}
	repo := NewUploadSessionRepo(db)
	if err := repo.Create(session); err != nil {
		t.Fatalf("create error = %v", err)
	}
	assertTenantSQL(t, capture, "tenant_id")
	if !strings.Contains(capture.query, "release_id") {
		t.Fatalf("create query missing release_id: %s", capture.query)
	}
	found := false
	for _, arg := range capture.args {
		if arg.Value == "tenant-a" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("create should include tenant_id arg: %+v", capture.args)
	}
}

func TestArtifactRepoReleaseAndUnassignedListsAreTenantScoped(t *testing.T) {
	db, capture := captureDB()
	defer db.Close()
	repo := NewArtifactRepo(db)

	if _, err := repo.ListUnassigned("tenant-a", 100, 0); err != nil {
		t.Fatal(err)
	}
	assertTenantSQL(t, capture, "a.tenant_id=$1")
	if !strings.Contains(capture.query, "NOT EXISTS") || !strings.Contains(capture.query, "release_artifacts") {
		t.Fatalf("unassigned query does not exclude release artifacts: %s", capture.query)
	}

	if _, err := repo.ListByRelease("release-a", "tenant-a"); err != nil {
		t.Fatal(err)
	}
	assertTenantSQL(t, capture, "pub.tenant_id=$2")
	for _, fragment := range []string{"ra.release_id=$1", "a.tenant_id=$2", "ORDER BY ra.position"} {
		if !strings.Contains(capture.query, fragment) {
			t.Fatalf("release artifact query missing %q: %s", fragment, capture.query)
		}
	}
}

func TestArtifactRepoIsTenantScoped(t *testing.T) {
	db, capture := captureDB()
	defer db.Close()

	repo := NewArtifactRepo(db)
	artifact := &appstore.ArtifactDescriptor{
		ID: "artifact-a", TenantID: "tenant-a", Name: "test.jar", ArtifactType: appstore.ArtJAR,
		MediaType: "application/java-archive", Repository: "hnb/jars",
		Digest:             "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		VerificationStatus: appstore.ArtifactVerified, LifecycleState: appstore.ArtifactActive,
	}
	if err := repo.Create(artifact); err != nil {
		t.Fatalf("create error = %v", err)
	}
	assertTenantSQL(t, capture, "tenant_id")

	if _, err := repo.Get("artifact-a", "tenant-a"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("get error = %v", err)
	}
	assertTenantSQL(t, capture, "tenant_id=$2")

	artifacts, err := repo.List("tenant-a", 100, 0)
	if err != nil {
		t.Fatalf("list error = %v", err)
	}
	if len(artifacts) != 0 {
		t.Fatalf("expected empty artifact list, got %d", len(artifacts))
	}
	assertTenantSQL(t, capture, "tenant_id=$1")
}

func TestUploadSessionRepoGetCarriesTenant(t *testing.T) {
	db, capture := captureDB()
	defer db.Close()

	if _, err := NewUploadSessionRepo(db).Get("session-a", "tenant-a"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("get error = %v", err)
	}
	assertTenantSQL(t, capture, "tenant_id=$2")
}

func assertTenantSQL(t *testing.T, capture *captureConnector, fragment string) {
	t.Helper()
	if !strings.Contains(capture.query, fragment) {
		t.Fatalf("query %q does not contain %q", capture.query, fragment)
	}
	found := false
	for _, argument := range capture.args {
		if argument.Value == "tenant-a" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("query tenant argument missing: %+v", capture.args)
	}
}
