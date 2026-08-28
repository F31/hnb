package handler

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestStorageDesiredStoreScopesReadsByTenant(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta("FROM storage_backends WHERE tenant_id=$1 AND id=$2")).
		WithArgs("tenant-a", "32684d2c-fca8-4f28-a946-fb267363fd6c").
		WillReturnError(sqlmock.ErrCancelled)
	_, err = NewPostgresStorageDesiredStore(db).GetBackend(context.Background(), "tenant-a", "32684d2c-fca8-4f28-a946-fb267363fd6c")
	if err == nil || mock.ExpectationsWereMet() != nil {
		t.Fatalf("err=%v expectations=%v", err, mock.ExpectationsWereMet())
	}
}

func TestStorageDesiredStoreLoadsProjectedBindingCondition(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	columns := []string{"id", "tenant_id", "offering_id", "offering_version", "target_id", "storage_class_name", "storage_class_uid", "storage_class_resource_version", "sync_state", "is_default", "source", "observed_at", "freshness", "topology", "conditions", "version", "created_at", "updated_at"}
	mock.ExpectQuery(`FROM storage_class_bindings WHERE tenant_id=\$1 AND id=\$2`).
		WithArgs("tenant-a", "42684d2c-fca8-4f28-a946-fb267363fd6c").
		WillReturnRows(sqlmock.NewRows(columns).AddRow("42684d2c-fca8-4f28-a946-fb267363fd6c", "tenant-a", "52684d2c-fca8-4f28-a946-fb267363fd6c", 1, "62684d2c-fca8-4f28-a946-fb267363fd6c", "fast", "desired-uid", "1", "Drifted", false, "runtime_target_storage_inventory", now, "Fresh", []byte(`{}`), []byte(`[{"type":"Drifted","status":"True","reason":"StorageClassUIDChanged"}]`), 3, now, now))
	item, err := NewPostgresStorageDesiredStore(db).GetBinding(context.Background(), "tenant-a", "42684d2c-fca8-4f28-a946-fb267363fd6c")
	if err != nil || len(item.Conditions) != 1 || item.Conditions[0]["reason"] != "StorageClassUIDChanged" {
		t.Fatalf("item=%+v err=%v", item, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStorageDesiredStoreDistinguishesStaleVersionFromMissing(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(`UPDATE storage_backends SET[\s\S]*WHERE tenant_id=\$1 AND id=\$2 AND version=\$3`).
		WithArgs("tenant-a", "32684d2c-fca8-4f28-a946-fb267363fd6c", int64(4), "generic-csi", "1.0.0", "array-a", "Array A", "", nil, nil, nil, nil, []byte(`{}`)).
		WillReturnError(sqlmock.ErrCancelled)
	store := NewPostgresStorageDesiredStore(db)
	_, err = store.UpdateBackend(context.Background(), "tenant-a", "32684d2c-fca8-4f28-a946-fb267363fd6c", 4, storageBackendInput{ProviderType: "generic-csi", ProviderSchemaVersion: "1.0.0", BackendID: "array-a", DisplayName: "Array A"})
	if err == nil {
		t.Fatal("expected update error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}

	db2, mock2, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	mock2.ExpectExec(regexp.QuoteMeta("DELETE FROM storage_class_bindings WHERE tenant_id=$1 AND id=$2 AND version=$3")).
		WithArgs("tenant-a", "42684d2c-fca8-4f28-a946-fb267363fd6c", int64(3)).WillReturnResult(sqlmock.NewResult(0, 0))
	mock2.ExpectQuery(regexp.QuoteMeta("SELECT EXISTS(SELECT 1 FROM storage_class_bindings WHERE tenant_id=$1 AND id=$2)")).
		WithArgs("tenant-a", "42684d2c-fca8-4f28-a946-fb267363fd6c").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	err = NewPostgresStorageDesiredStore(db2).DeleteBinding(context.Background(), "tenant-a", "42684d2c-fca8-4f28-a946-fb267363fd6c", 3)
	if !errors.Is(err, errStorageVersionConflict) {
		t.Fatalf("err=%v", err)
	}
	if err := mock2.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStorageDesiredStoreValidatesTenantOwnedSecretMetadata(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(`SELECT EXISTS`).WithArgs("tenant-a", "platform-secrets", "tenant:tenant-a", "nfs-primary", "3").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	err = NewPostgresStorageDesiredStore(db).ValidateSecretReference(context.Background(), "tenant-a", secretReference{Provider: "platform-secrets", Scope: "tenant:tenant-a", Name: "nfs-primary", Version: "3"})
	if err != nil || mock.ExpectationsWereMet() != nil {
		t.Fatalf("err=%v expectations=%v", err, mock.ExpectationsWereMet())
	}
}
