package iam

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestRecordKeyManifestIsIdempotentAndStoresNoPrivateFields(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC().Round(time.Microsecond)
	manifest := KeyManifest{Issuer: "https://issuer.example", Generation: 9, ActiveKeyID: "k2", Keys: map[string]KeyManifestEntry{
		"k2": {PublicKeyPath: "/keys/k2-public.pem", Status: KeyActive, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour)},
	}}
	store := NewIAMDBStore(db, manifest.Issuer)

	const id = "00000000-0000-0000-0000-000000000009"
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtext($1))`)).WithArgs(manifest.Issuer).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT COALESCE\\(MAX\\(version\\), 0\\)").WithArgs(manifest.Issuer).WillReturnRows(sqlmock.NewRows([]string{"generation"}).AddRow(0))
	mock.ExpectQuery("SELECT id::text, status, version, verification_key_ref, not_before, not_after").WithArgs(manifest.Issuer, "k2").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("INSERT INTO signing_key_metadata").
		WithArgs(manifest.Issuer, "k2", "manifest:9:k2", "/keys/k2-public.pem", KeyActive, uint64(9), manifest.Keys["k2"].NotBefore, manifest.Keys["k2"].NotAfter).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(id))
	mock.ExpectExec("INSERT INTO signing_key_lifecycle_events").WithArgs(id, "created").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO signing_key_lifecycle_events").WithArgs(id, "activated").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	if err := store.RecordKeyManifest(context.Background(), manifest); err != nil {
		t.Fatal(err)
	}

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtext($1))`)).WithArgs(manifest.Issuer).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT COALESCE\\(MAX\\(version\\), 0\\)").WithArgs(manifest.Issuer).WillReturnRows(sqlmock.NewRows([]string{"generation"}).AddRow(9))
	mock.ExpectQuery("SELECT id::text, status, version, verification_key_ref, not_before, not_after").WithArgs(manifest.Issuer, "k2").
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "version", "verification_key_ref", "not_before", "not_after"}).
			AddRow(id, "active", int64(9), "/keys/k2-public.pem", manifest.Keys["k2"].NotBefore, manifest.Keys["k2"].NotAfter))
	mock.ExpectCommit()
	if err := store.RecordKeyManifest(context.Background(), manifest); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// A manifest carries nanosecond timestamps while PostgreSQL TIMESTAMPTZ keeps
// only microseconds. Re-verifying an already-recorded manifest must still
// succeed, otherwise an apiserver can never restart against its own key.
func TestRecordKeyManifestAcceptsNanosecondManifestAfterDatabaseRoundTrip(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	nanosecond := time.Date(2026, 10, 13, 3, 54, 22, 688204943, time.UTC)
	manifest := KeyManifest{Issuer: "https://issuer.example", Generation: 9, ActiveKeyID: "k2", Keys: map[string]KeyManifestEntry{
		"k2": {PublicKeyPath: "/keys/k2-public.pem", Status: KeyActive, NotBefore: nanosecond, NotAfter: nanosecond.Add(time.Hour)},
	}}
	store := NewIAMDBStore(db, manifest.Issuer)

	// First record: the manifest is inserted at database precision.
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtext($1))`)).WithArgs(manifest.Issuer).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT COALESCE\\(MAX\\(version\\), 0\\)").WithArgs(manifest.Issuer).WillReturnRows(sqlmock.NewRows([]string{"generation"}).AddRow(0))
	mock.ExpectQuery("SELECT id::text, status, version, verification_key_ref, not_before, not_after").WithArgs(manifest.Issuer, "k2").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery("INSERT INTO signing_key_metadata").
		WithArgs(manifest.Issuer, "k2", "manifest:9:k2", "/keys/k2-public.pem", KeyActive, uint64(9),
			nanosecond.Round(time.Microsecond), nanosecond.Add(time.Hour).Round(time.Microsecond)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("00000000-0000-0000-0000-000000000009"))
	mock.ExpectExec("INSERT INTO signing_key_lifecycle_events").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO signing_key_lifecycle_events").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	if err := store.RecordKeyManifest(context.Background(), manifest); err != nil {
		t.Fatalf("first record: %v", err)
	}

	// Restart: the stored row holds the value PostgreSQL kept (microseconds).
	const id = "00000000-0000-0000-0000-000000000009"
	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtext($1))`)).WithArgs(manifest.Issuer).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT COALESCE\\(MAX\\(version\\), 0\\)").WithArgs(manifest.Issuer).WillReturnRows(sqlmock.NewRows([]string{"generation"}).AddRow(9))
	mock.ExpectQuery("SELECT id::text, status, version, verification_key_ref, not_before, not_after").WithArgs(manifest.Issuer, "k2").
		WillReturnRows(sqlmock.NewRows([]string{"id", "status", "version", "verification_key_ref", "not_before", "not_after"}).
			AddRow(id, "active", int64(9), "/keys/k2-public.pem",
				nanosecond.Round(time.Microsecond), nanosecond.Add(time.Hour).Round(time.Microsecond)))
	mock.ExpectCommit()
	if err := store.RecordKeyManifest(context.Background(), manifest); err != nil {
		t.Fatalf("restart re-verification rejected a database round-tripped manifest: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
