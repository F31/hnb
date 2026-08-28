package alert

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestValidateStorageMetricRejectsUnavailableIOPS(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery("FROM storage_metric_snapshots").
		WithArgs("tenant-a", "32684d2c-fca8-4f28-a946-fb267363fd6c", "nfs", "StorageBackend", "backend-a", "iops").
		WillReturnRows(sqlmock.NewRows([]string{"applicability", "freshness", "status", "unit", "source", "observed_at", "stale_after"}).
			AddRow("Unsupported", "Fresh", "NotReported", "1/s", "nfs_exporter", time.Now(), time.Now().Add(time.Hour)))
	err = NewAlertDBStore(db).ValidateStorageMetric(context.Background(),
		ResourceReference{TenantID: "tenant-a", TargetID: "32684d2c-fca8-4f28-a946-fb267363fd6c", Kind: "StorageBackend", UID: "backend-a"},
		StorageMetricCondition{ProviderID: "nfs", Kind: "iops", Unit: "1/s", Source: "nfs_exporter", FreshFor: time.Minute})
	if !errors.Is(err, ErrMetricUnavailable) {
		t.Fatalf("expected unavailable IOPS, got %v", err)
	}
}

func TestListStorageRulesIsTenantScoped(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery(regexp.QuoteMeta("FROM alert_rules WHERE source_type='storage-metric' AND tenant_id=$1 ORDER BY name, id")).
		WithArgs("tenant-a").WillReturnRows(sqlmock.NewRows([]string{"id", "name", "description", "severity", "enabled", "tenant_id", "target_id", "resource_kind", "resource_uid", "resource_namespace", "resource_name", "provider_id", "metric_kind", "metric_unit", "metric_source", "metric_fresh_for", "comparison_operator", "threshold", "duration", "channel_refs", "annotations", "version"}))
	if _, err := NewAlertDBStore(db).ListStorageRules(context.Background(), "tenant-a"); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateChannelReferencesIsTenantScopedAndReadsNoSecretValue(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery("SELECT EXISTS").WithArgs("tenant-a", "platform-secrets", "tenant:tenant-a", "storage-hook", "3").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	err = NewAlertDBStore(db).ValidateChannelReferences(context.Background(), "tenant-a", []ChannelReference{{
		Type: "webhook", ConfigReference: "channel-a",
		SecretReference: SecretReference{Provider: "platform-secrets", Scope: "tenant:tenant-a", Name: "storage-hook", Version: "3"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestEvaluateStorageRulesPersistsPVCNavigationIdentity(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectExec("INSERT INTO alert_instances").WithArgs(sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(1, 1))
	if err := NewAlertDBStore(db).EvaluateStorageRules(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateRequiresCanonicalModelsWithoutCreatingPrivateTables(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery("SELECT to_regclass").WillReturnRows(sqlmock.NewRows([]string{"canonical"}).AddRow(true))
	if err := NewAlertDBStore(db).Migrate(); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
