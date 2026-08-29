package seed

import (
	"database/sql"
	"os"
	"strings"
	"testing"

	appstorestore "github.com/F31/hnb/pkg/appstore/store"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

func randomSuffix() string {
	return strings.ReplaceAll(uuid.NewString()[:8], "-", "")
}

func TestSeedPluginCatalogIdempotent(t *testing.T) {
	dsn := os.Getenv("HNB_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("HNB_TEST_POSTGRES_DSN is not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	tenantID := "plugin-catalog-" + randomSuffix()
	if _, err := db.Exec(`INSERT INTO tenants (id, name, display_name) VALUES ($1, $1, $1)`, tenantID); err != nil {
		t.Fatal(err)
	}
	// The seed uses the tenant's default publisher; clean up whatever it creates.
	t.Cleanup(func() {
		rows, err := db.Query(`SELECT id FROM publishers WHERE tenant_id=$1`, tenantID)
		var pubIDs []string
		if err == nil {
			for rows.Next() {
				var id string
				if rows.Scan(&id) == nil {
					pubIDs = append(pubIDs, id)
				}
			}
			rows.Close()
		}
		_, _ = db.Exec(`DELETE FROM releases WHERE product_id IN (SELECT p.id FROM products p JOIN publishers pub ON pub.id=p.publisher_id WHERE pub.tenant_id=$1)`, tenantID)
		_, _ = db.Exec(`DELETE FROM products WHERE publisher_id IN (SELECT id FROM publishers WHERE tenant_id=$1)`, tenantID)
		for _, id := range pubIDs {
			_, _ = db.Exec(`DELETE FROM publishers WHERE id=$1`, id)
		}
		_, _ = db.Exec(`DELETE FROM tenants WHERE id=$1`, tenantID)
	})

	pubRepo := appstorestore.NewPublisherRepo(db)
	prodRepo := appstorestore.NewProductRepo(db)
	relRepo := appstorestore.NewReleaseRepo(db)

	if err := SeedPluginCatalog(pubRepo, prodRepo, relRepo, tenantID); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	// Second run must be a no-op (idempotent).
	if err := SeedPluginCatalog(pubRepo, prodRepo, relRepo, tenantID); err != nil {
		t.Fatalf("second seed: %v", err)
	}

	// Publisher created.
	pub, err := pubRepo.DefaultPublisher(tenantID)
	if err != nil {
		t.Fatalf("default publisher: %v", err)
	}
	if pub.Name != "hnb-official" {
		t.Fatalf("publisher name = %q, want hnb-official", pub.Name)
	}

	// Every catalog entry has a public product with a tagged label and a release.
	// Use the tenant-scoped List (Search scope=public would include other tenants).
	products, err := prodRepo.List(pub.ID, tenantID)
	if err != nil {
		t.Fatal(err)
	}
	productByName := map[string]string{} // name -> id
	for _, p := range products {
		productByName[p.Name] = p.ID
	}
	for _, entry := range Catalog() {
		id, ok := productByName[entry.Name]
		if !ok {
			t.Errorf("product %q missing after seed", entry.Name)
			continue
		}
		if entry.Version != "" {
			rels, err := relRepo.ListByProduct(id, tenantID)
			if err != nil || len(rels) == 0 {
				t.Errorf("product %q has no release", entry.Name)
				continue
			}
			var published bool
			for _, rel := range rels {
				if rel.Version == entry.Version && string(rel.Status) == "published" {
					published = true
				}
			}
			if !published {
				t.Errorf("product %q missing published release %s", entry.Name, entry.Version)
			}
		}
	}
}
