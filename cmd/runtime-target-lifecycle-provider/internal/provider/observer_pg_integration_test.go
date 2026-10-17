package provider

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestObserverRegistryPGReplay(t *testing.T) {
	dsn := os.Getenv("HNB_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("HNB_TEST_POSTGRES_DSN is not set")
	}
	base, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer base.Close()
	schema := "observer_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = base.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	defer base.Exec(`DROP SCHEMA ` + schema + ` CASCADE`)
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE runtime_targets (id uuid PRIMARY KEY, tenant_id text, target_type text,
		observation_source text, observation_source_id text, observation_generation bigint,
		observation_revision bigint, updated_at timestamptz);
		INSERT INTO runtime_targets VALUES ('515eba09-0a41-5b92-b972-69af1f0f655c','tenant-a','kubernetes',NULL,NULL,0,0,now())`)
	if err != nil {
		t.Fatal(err)
	}
	registry := NewPGObserverRegistry(db)
	ctx := context.Background()
	id := "515eba09-0a41-5b92-b972-69af1f0f655c"
	if err := registry.Register(ctx, "tenant-a", id, "KubernetesTarget", "Agent"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE runtime_targets SET observation_revision=7`); err != nil {
		t.Fatal(err)
	}
	read := func() string {
		var state string
		if err := db.QueryRow(`SELECT concat(observation_source,':',observation_source_id,':',observation_generation,':',observation_revision,':',updated_at) FROM runtime_targets`).Scan(&state); err != nil {
			t.Fatal(err)
		}
		return state
	}
	before := read()
	if err := registry.Register(ctx, "tenant-a", id, "KubernetesTarget", "Agent"); err != nil {
		t.Fatal(err)
	}
	if after := read(); after != before {
		t.Fatalf("register replay changed observer: before=%s after=%s", before, after)
	}
	if err := registry.Unregister(ctx, "tenant-a", id); err != nil {
		t.Fatal(err)
	}
	var generation int
	if err := db.QueryRow(`SELECT observation_generation FROM runtime_targets`).Scan(&generation); err != nil || generation != 2 {
		t.Fatalf("unregister generation=%d err=%v", generation, err)
	}
	before = read()
	if err := registry.Unregister(ctx, "tenant-a", id); err != nil {
		t.Fatal(err)
	}
	if after := read(); after != before {
		t.Fatalf("unregister replay changed observer: before=%s after=%s", before, after)
	}
	if err := registry.Register(ctx, "wrong-tenant", id, "KubernetesTarget", "Agent"); err == nil {
		t.Fatal("wrong tenant accepted")
	}
}
