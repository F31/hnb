package toolkit

import (
	"context"
	"database/sql"
	"time"

	"github.com/F31/hnb/pkg/db"
)

// The migration engine lives in pkg/db so the platform services and the
// installer share one implementation. This file re-exports the types and
// methods the installer tooling needs, keeping a single source of truth.

type MigrationOwner = db.MigrationOwner

const (
	OwnerPlatform = db.OwnerPlatform
	OwnerMarket   = db.OwnerMarket
	OwnerArtifact = db.OwnerArtifact
)

type MigrationFile = db.MigrationFile
type MigrationRecord = db.MigrationRecord
type MigrationOutcome = db.MigrationOutcome

const MigrationLockTimeout = db.MigrationLockTimeout

// Migrator wraps pkg/db.Migrator for the installer's schema gating needs.
type Migrator struct {
	DB    *sql.DB
	Owner MigrationOwner
	Now   func() time.Time
}

func (m Migrator) wrap() db.Migrator {
	return db.Migrator{DB: m.DB, Owner: m.Owner, Now: m.Now}
}

func (m Migrator) LoadMigrations(dir string) ([]MigrationFile, error) {
	return db.LoadMigrations(dir)
}

func (m Migrator) Verify(ctx context.Context, files []MigrationFile) ([]string, []MigrationRecord, error) {
	return m.wrap().Verify(ctx, files)
}

func (m Migrator) Apply(ctx context.Context, files []MigrationFile) (*MigrationOutcome, error) {
	return m.wrap().Apply(ctx, files)
}

func (m Migrator) VerifySchemaForService(ctx context.Context, files []MigrationFile) (bool, []string, error) {
	return m.wrap().VerifySchemaForService(ctx, files)
}

func (m Migrator) Baseline(ctx context.Context, files []MigrationFile, manifest *db.MigrationBaseline) error {
	return m.wrap().Baseline(ctx, files, manifest)
}

func (m Migrator) InspectSchema(ctx context.Context) (string, string, error) {
	return m.wrap().InspectSchema(ctx)
}

// LoadMigrations reads forward migrations from a directory.
func LoadMigrations(dir string) ([]MigrationFile, error) {
	return db.LoadMigrations(dir)
}
