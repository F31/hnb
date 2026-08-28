package store

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("HNB_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("HNB_TEST_POSTGRES_DSN not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestClusterStore_ListActive(t *testing.T) {
	db := openTestDB(t)
	store := NewClusterStore(db)
	ctx := context.Background()

	clusters, err := store.ListActive(ctx)
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	t.Logf("active clusters: %d", len(clusters))
	for _, c := range clusters {
		t.Logf("  %s (%s) - %s", c.Name, c.APIEndpoint, c.Status)
	}
}

func TestClusterStore_ListAll(t *testing.T) {
	db := openTestDB(t)
	store := NewClusterStore(db)
	ctx := context.Background()

	clusters, err := store.ListAll(ctx)
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	t.Logf("total clusters: %d", len(clusters))
}

func TestClusterStore_UpdateStatus(t *testing.T) {
	db := openTestDB(t)
	store := NewClusterStore(db)
	ctx := context.Background()

	clusters, err := store.ListActive(ctx)
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(clusters) == 0 {
		t.Skip("no clusters to update")
	}

	c := clusters[0]
	err = store.UpdateStatus(ctx, c.ID, "healthy")
	if err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}

	updated, err := store.ListAll(ctx)
	if err != nil {
		t.Fatalf("ListAll after update: %v", err)
	}
	for _, u := range updated {
		if u.ID == c.ID {
			if u.Status != "healthy" {
				t.Errorf("expected status healthy, got %s", u.Status)
			}
			break
		}
	}

	err = store.UpdateStatus(ctx, c.ID, c.Status)
	if err != nil {
		t.Fatalf("restore status: %v", err)
	}
}

func TestClusterStore_RecordHeartbeat(t *testing.T) {
	db := openTestDB(t)
	store := NewClusterStore(db)
	ctx := context.Background()

	clusters, err := store.ListActive(ctx)
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}
	if len(clusters) == 0 {
		t.Skip("no clusters for heartbeat")
	}

	c := clusters[0]
	err = store.RecordHeartbeat(ctx, c.ID, "healthy", "v1.30.0", 3, map[string]any{"cpu": 8, "mem": "32Gi"})
	if err != nil {
		t.Fatalf("RecordHeartbeat: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	updated, err := store.ListAll(ctx)
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	for _, u := range updated {
		if u.ID == c.ID {
			if u.LastHeartbeat == nil {
				t.Errorf("expected last_heartbeat to be set")
			}
			break
		}
	}
}