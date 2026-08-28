package main

import (
	"database/sql"
	"errors"
	"os"
	"testing"

	"github.com/F31/hnb/pkg/appstore"
	appstorestore "github.com/F31/hnb/pkg/appstore/store"
	"github.com/google/uuid"
)

func TestAppMarketRepositoriesHideForeignTenantUUIDs(t *testing.T) {
	dsn := os.Getenv("HNB_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("HNB_TEST_POSTGRES_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	tenantA, tenantB := "market-a-"+uuid.NewString(), "market-b-"+uuid.NewString()
	publisherID, productID, releaseID, applicationID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	for _, tenantID := range []string{tenantA, tenantB} {
		if _, err := db.Exec(`INSERT INTO tenants (id, name, display_name) VALUES ($1,$1,$1)`, tenantID); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM applications WHERE id=$1`, applicationID)
		_, _ = db.Exec(`DELETE FROM releases WHERE id=$1`, releaseID)
		_, _ = db.Exec(`DELETE FROM products WHERE id=$1`, productID)
		_, _ = db.Exec(`DELETE FROM publishers WHERE id=$1`, publisherID)
		_, _ = db.Exec(`DELETE FROM tenants WHERE id IN ($1,$2)`, tenantA, tenantB)
	})
	if _, err := db.Exec(`INSERT INTO publishers (id, tenant_id, name, display_name) VALUES ($1,$2,'publisher','Publisher')`, publisherID, tenantA); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO products (id, publisher_id, name, display_name, category) VALUES ($1,$2,'product','Product','application')`, productID, publisherID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO releases (id, product_id, version, manifest_digest, created_by) VALUES ($1,$2,'1.0.0',$3,'subject')`, releaseID, productID, "sha256:"+uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO applications (id, tenant_id, product_id, release_id, name) VALUES ($1,$2,$3,$4,'app')`, applicationID, tenantA, productID, releaseID); err != nil {
		t.Fatal(err)
	}

	publishers := appstorestore.NewPublisherRepo(db)
	products := appstorestore.NewProductRepo(db)
	releases := appstorestore.NewReleaseRepo(db)
	applications := appstorestore.NewApplicationRepo(db)
	for name, call := range map[string]func() error{
		"publisher get":      func() error { _, err := publishers.Get(publisherID, tenantB); return err },
		"product get":        func() error { _, err := products.Get(productID, tenantB); return err },
		"release get":        func() error { _, err := releases.Get(releaseID, tenantB); return err },
		"release publish":    func() error { return releases.Publish(releaseID, tenantB) },
		"application get":    func() error { _, err := applications.Get(applicationID, tenantB); return err },
		"application update": func() error { return applications.UpdateStatus(applicationID, tenantB, appstore.AppUninstalling) },
		"application delete": func() error { return applications.Delete(applicationID, tenantB) },
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("error = %v, want sql.ErrNoRows", err)
			}
		})
	}

	if err := products.Create(&appstore.Product{ID: uuid.NewString(), PublisherID: publisherID, Name: "foreign", DisplayName: "Foreign", Category: appstore.CatApplication, Status: appstore.ProdDraft}, tenantB); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("foreign product association error = %v", err)
	}
	if err := releases.Publish(releaseID, tenantA); err != nil {
		t.Fatalf("tenant publish (without releases.updated_at) failed: %v", err)
	}
	application, err := applications.Get(applicationID, tenantA)
	if err != nil || application.Status != appstore.AppDeploying {
		t.Fatalf("foreign mutation changed application: %+v, %v", application, err)
	}
}
