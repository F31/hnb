package handler

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPostgresStorageStoreInventoryReadsTenantBoundProjections(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewPostgresStorageStore(db)
	now := time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`SELECT resource_kind, resource_uid, resource_version, name,[\s\S]*WHERE tenant_id = \$1 AND target_id = \$2 AND deleted_at IS NULL`).
		WithArgs("tenant-a", storageTestTarget, "StorageClass", "fast", "driver.csi", "Fresh", 20, 20).
		WillReturnRows(sqlmock.NewRows([]string{"resource_kind", "resource_uid", "resource_version", "name", "namespace", "driver_name", "source", "observed_at", "stale_after", "attributes"}).
			AddRow("StorageClass", "uid-a", "3", "fast", "", "driver.csi", "agent", now, now.Add(time.Hour), []byte(`{"provisioner":"driver.csi"}`)))
	mock.ExpectQuery(`SELECT DISTINCT driver_name[\s\S]*runtime_target_storage_driver_evidence[\s\S]*tenant_id = \$1 AND target_id = \$2`).
		WithArgs("tenant-a", storageTestTarget).
		WillReturnRows(sqlmock.NewRows([]string{"driver_name"}).AddRow("driver.csi"))
	mock.ExpectQuery(`SELECT status, api_version, source, observed_at, stale_after[\s\S]*runtime_target_storage_snapshot_api[\s\S]*tenant_id = \$1 AND target_id = \$2`).
		WithArgs("tenant-a", storageTestTarget).
		WillReturnRows(sqlmock.NewRows([]string{"status", "api_version", "source", "observed_at", "stale_after"}).AddRow("Installed", "v1", "agent", now, now.Add(time.Hour)))

	items, registrations, snapshot, err := store.Inventory(context.Background(), "tenant-a", storageTestTarget, storageInventoryQuery{
		Kind: "StorageClass", Name: "fast", DriverName: "driver.csi", Freshness: "Fresh", Limit: 20, Offset: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || !registrations["driver.csi"] || snapshot == nil || snapshot.Status != "Installed" {
		t.Fatalf("items=%+v registrations=%+v snapshot=%+v", items, registrations, snapshot)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresStorageStoreTargetOwnershipRequiresTenantAndActiveTarget(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := NewPostgresStorageStore(db)
	mock.ExpectQuery(`SELECT EXISTS \(`).WithArgs(storageTestTarget, "tenant-a").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	owned, err := store.TargetOwned(context.Background(), "tenant-a", storageTestTarget)
	if err != nil || owned {
		t.Fatalf("owned=%v err=%v", owned, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
