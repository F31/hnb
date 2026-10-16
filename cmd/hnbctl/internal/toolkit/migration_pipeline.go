package toolkit

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// MigrationPlane pairs one owning plane with its migration material and the DSN
// to apply against. The DSN is used only to open the connection and is never
// logged, returned or retained in a report.
type MigrationPlane struct {
	Owner         MigrationOwner
	MigrationsDir string
	DSN           string
}

// MigrationOutcomeReport summarizes one plane's apply without exposing secrets.
type MigrationOutcomeReport struct {
	Owner       string `json:"owner"`
	Applied     int    `json:"applied"`
	Skipped     int    `json:"skipped"`
	SchemaAfter string `json:"schemaAfter,omitempty"`
}

// DBConnector opens a database from a DSN. Injected so callers control the
// driver and lifecycle; tests can avoid a real database.
type DBConnector func(dsn string) (*sql.DB, error)

// DefaultDBConnector opens a PostgreSQL connection. It does not connect eagerly;
// the first migration statement establishes the connection.
func DefaultDBConnector(dsn string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("cannot open plane database")
	}
	db.SetMaxOpenConns(2)
	return db, nil
}

// MigrationApplier applies one plane's migrations. Injected so the orchestration
// is testable without a database, mirroring the cluster applier seam.
type MigrationApplier func(ctx context.Context, plane MigrationPlane) (MigrationOutcomeReport, error)

// DefaultMigrationApplier applies a plane's forward migrations under the shared
// pkg/db migrator (owner-scoped advisory lock).
func DefaultMigrationApplier(ctx context.Context, plane MigrationPlane) (MigrationOutcomeReport, error) {
	return applyMigrationPlane(ctx, plane, DefaultDBConnector)
}

func applyMigrationPlane(ctx context.Context, plane MigrationPlane, connect DBConnector) (MigrationOutcomeReport, error) {
	if plane.Owner == "" || plane.MigrationsDir == "" || plane.DSN == "" {
		return MigrationOutcomeReport{}, fmt.Errorf("MIGRATION_INPUT_INVALID: owner, migrations directory and DSN are required")
	}
	files, err := LoadMigrations(plane.MigrationsDir)
	if err != nil {
		return MigrationOutcomeReport{}, fmt.Errorf("MIGRATION_MATERIAL_INVALID: %s", plane.Owner)
	}
	db, err := connect(plane.DSN)
	if err != nil {
		return MigrationOutcomeReport{}, fmt.Errorf("MIGRATION_CONNECT_FAILED: %s", plane.Owner)
	}
	defer db.Close()
	outcome, err := (Migrator{DB: db, Owner: plane.Owner}).Apply(ctx, files)
	if err != nil {
		// The migrator error is reported by class only: it must never carry the DSN.
		return MigrationOutcomeReport{}, fmt.Errorf("MIGRATION_APPLY_FAILED: %s", plane.Owner)
	}
	return MigrationOutcomeReport{Owner: string(plane.Owner), Applied: len(outcome.Applied), Skipped: len(outcome.Skipped), SchemaAfter: outcome.SchemaAfter}, nil
}

// RunDatabaseMigrations applies each plane's migrations in order and fails closed
// on the first plane error. A partial result is returned alongside the error so
// the caller can record which planes were already applied.
func RunDatabaseMigrations(ctx context.Context, planes []MigrationPlane, applier MigrationApplier) ([]MigrationOutcomeReport, error) {
	if applier == nil {
		applier = DefaultMigrationApplier
	}
	var reports []MigrationOutcomeReport
	for _, plane := range planes {
		report, err := applier(ctx, plane)
		if err != nil {
			return reports, err
		}
		reports = append(reports, report)
	}
	return reports, nil
}

// MigrationOwnerForPlane maps a configured plane name to its owning migration
// set. Unknown planes are refused rather than defaulted to the platform owner.
func MigrationOwnerForPlane(name string) (MigrationOwner, bool) {
	switch name {
	case "platform":
		return OwnerPlatform, true
	case "market":
		return OwnerMarket, true
	case "artifact":
		return OwnerArtifact, true
	default:
		return "", false
	}
}

// RunInstallMigrations is the gated migration stage. It refuses unless the
// delivery evidence gate passes, then applies each plane's migrations and
// records the database-migration phase. It never runs while the gate refuses.
func RunInstallMigrations(ctx context.Context, store RunStore, writer string, epoch int64, cfg *DeploymentConfig,
	planes []MigrationPlane, applier MigrationApplier, evidence *EvidenceMatrix) ([]MigrationOutcomeReport, error) {
	if cfg == nil {
		return nil, fmt.Errorf("PLATFORM_NOT_CERTIFIED: no configuration")
	}
	if reasons := EvidenceGate(cfg, evidence); len(reasons) > 0 {
		return nil, fmt.Errorf("PLATFORM_NOT_CERTIFIED: %s", joinReasons(reasons))
	}
	if len(planes) == 0 {
		return nil, fmt.Errorf("MIGRATION_INPUT_INVALID: no planes declared")
	}
	var reports []MigrationOutcomeReport
	budget := cfg.Spec.Timeouts.Readiness
	if budget <= 0 {
		budget = 10 * time.Minute
	}
	err := store.RunPhaseWithTimeout(ctx, writer, epoch, PhaseDatabaseMigrate, budget, nil, nil, func(ctx context.Context) error {
		var applyErr error
		reports, applyErr = RunDatabaseMigrations(ctx, planes, applier)
		return applyErr
	})
	return reports, err
}
