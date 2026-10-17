package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"fmt"
	"hash/adler32"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

// MigrationLockTimeout bounds how long an installer waits for another installer's
// migration lock. It never blocks forever: a stuck peer must surface as a
// conflict, not a silent hang.
const MigrationLockTimeout = 30 * time.Second

// MigrationOwner identifies a plane whose schema is migrated independently.
type MigrationOwner string

const (
	OwnerPlatform MigrationOwner = "platform"
	OwnerMarket   MigrationOwner = "market"
	OwnerArtifact MigrationOwner = "artifact"
)

// MigrationFile is one forward migration parsed from disk.
type MigrationFile struct {
	Version  int64
	Name     string
	Checksum string
	SQL      string
}

// MigrationRecord is what the ledger stores for an applied migration.
type MigrationRecord struct {
	Version   int64
	Name      string
	Checksum  string
	AppliedAt time.Time
	// For Origin=baseline, AppliedAt is the adoption time, not a claim about
	// when legacy SQL originally executed (which may be unknown).
	Origin     string
	ReviewedBy string
}

// MigrationOutcome reports what a migration run did.
type MigrationOutcome struct {
	Owner        MigrationOwner
	Applied      []MigrationRecord
	Skipped      []MigrationRecord
	SchemaBefore string
	SchemaAfter  string
}

// LoadMigrations reads forward migrations from a directory. Files are ordered by
// their numeric version prefix and the rollback twin of a migration is never
// treated as a forward step.
func LoadMigrations(dir string) ([]MigrationFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("migrations directory unavailable")
	}
	var files []MigrationFile
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".sql") || strings.HasSuffix(name, ".rollback.sql") {
			continue
		}
		prefix, _, ok := strings.Cut(name, "_")
		if !ok {
			return nil, fmt.Errorf("migration %q must be named <version>_<name>.sql", name)
		}
		version, err := strconv.ParseInt(prefix, 10, 64)
		if err != nil || version <= 0 {
			return nil, fmt.Errorf("migration %q must start with a positive numeric version", name)
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("migration %q is not readable", name)
		}
		sum := sha256.Sum256(data)
		files = append(files, MigrationFile{Version: version, Name: name, Checksum: hex.EncodeToString(sum[:]), SQL: string(data)})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Version < files[j].Version })
	for i := 1; i < len(files); i++ {
		if files[i].Version == files[i-1].Version {
			return nil, fmt.Errorf("migration version %d is declared twice", files[i].Version)
		}
	}
	return files, nil
}

// Migrator applies migrations for one owner under an advisory lock.
type Migrator struct {
	DB    *sql.DB
	Owner MigrationOwner
	// Now is injectable so tests can reason about ordering.
	Now  func() time.Time
	conn *sql.Conn
}

type migrationDB interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (m Migrator) executor() migrationDB {
	if m.conn != nil {
		return m.conn
	}
	return m.DB
}

func (m Migrator) validate() error {
	if m.DB == nil {
		return fmt.Errorf("migration database is required")
	}
	if m.Owner != OwnerPlatform && m.Owner != OwnerMarket && m.Owner != OwnerArtifact {
		return fmt.Errorf("unknown migration owner")
	}
	return nil
}

// A session-scoped PostgreSQL lock must remain on the same physical connection
// as every DDL/ledger operation, regardless of pool size or concurrent traffic.
func (m Migrator) locked(ctx context.Context) (Migrator, func(), error) {
	if err := m.validate(); err != nil {
		return m, nil, err
	}
	lockCtx, cancel := context.WithTimeout(ctx, MigrationLockTimeout)
	defer cancel()
	conn, err := m.DB.Conn(lockCtx)
	if err != nil {
		return m, nil, fmt.Errorf("cannot obtain migration connection")
	}
	for {
		var acquired bool
		err := conn.QueryRowContext(lockCtx, `SELECT pg_try_advisory_lock($1)`, m.lockKey()).Scan(&acquired)
		if err != nil {
			conn.Close()
			return m, nil, fmt.Errorf("cannot obtain owner migration lock")
		}
		if acquired {
			break
		}
		select {
		case <-lockCtx.Done():
			conn.Close()
			return m, nil, fmt.Errorf("another installer holds the %s migration lock", m.Owner)
		case <-time.After(10 * time.Millisecond):
		}
	}
	m.conn = conn
	return m, func() {
		cleanupCtx, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		var released bool
		if err := conn.QueryRowContext(cleanupCtx, `SELECT pg_advisory_unlock($1)`, m.lockKey()).Scan(&released); err != nil || !released {
			// Do not put a possibly locked session back into the pool.
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
		conn.Close()
	}, nil
}

func (m Migrator) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now().UTC()
}

func (m Migrator) ledgerName() string {
	// Identifiers are derived from a fixed owner set, never from user input.
	return string(m.Owner) + "_schema_migrations"
}

// lockKey derives a stable advisory lock key for the three validated owners.
func (m Migrator) lockKey() int32 {
	sum := adler32.Checksum([]byte("hnb-migration:" + string(m.Owner)))
	return int32(sum)
}

// schemaFingerprint binds a baseline to the public database's columns/types,
// nullability/defaults and indexes. It excludes only the engine's own ledgers;
// it does not infer historical SQL execution or inspect seed-data semantics.
func (m Migrator) schemaFingerprint(ctx context.Context) (string, error) {
	rows, err := m.executor().QueryContext(ctx,
		`SELECT table_name, column_name, data_type, udt_name, is_nullable, COALESCE(column_default,''), ordinal_position
		 FROM information_schema.columns WHERE table_schema = 'public'
		 AND table_name NOT IN ('platform_schema_migrations','market_schema_migrations','artifact_schema_migrations')
		 ORDER BY table_name, ordinal_position`)
	if err != nil {
		return "", fmt.Errorf("cannot read schema fingerprint")
	}
	defer rows.Close()
	hash := sha256.New()
	for rows.Next() {
		var table, column, typ, udt, nullable, defaultValue string
		var position int
		if err := rows.Scan(&table, &column, &typ, &udt, &nullable, &defaultValue, &position); err != nil {
			return "", fmt.Errorf("cannot read schema fingerprint")
		}
		fmt.Fprintf(hash, "%q:%q:%q:%q:%q:%q:%d;", table, column, typ, udt, nullable, defaultValue, position)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("cannot finish schema fingerprint")
	}
	rows.Close()
	indexes, err := m.executor().QueryContext(ctx, `SELECT tablename, indexname, indexdef FROM pg_indexes
	 WHERE schemaname='public' AND tablename NOT IN ('platform_schema_migrations','market_schema_migrations','artifact_schema_migrations') ORDER BY tablename,indexname`)
	if err != nil {
		return "", fmt.Errorf("cannot fingerprint schema indexes")
	}
	defer indexes.Close()
	for indexes.Next() {
		var table, name, definition string
		if err := indexes.Scan(&table, &name, &definition); err != nil {
			return "", fmt.Errorf("cannot fingerprint schema index")
		}
		fmt.Fprintf(hash, "%q:%q:%q;", table, name, definition)
	}
	if err := indexes.Err(); err != nil {
		return "", fmt.Errorf("cannot finish index fingerprint")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (m Migrator) ensureLedger(ctx context.Context) error {
	// The ledger lives in the owner's own schema boundary; identifiers come only
	// from the fixed owner set.
	_, err := m.executor().ExecContext(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			version      bigint PRIMARY KEY,
			name         text NOT NULL,
			checksum     char(64) NOT NULL,
			applied_at   timestamptz NOT NULL,
			origin text NOT NULL DEFAULT 'executed',
			reviewed_by text NOT NULL DEFAULT '',
			legacy_name text NOT NULL DEFAULT '',
			baseline_schema_fingerprint text NOT NULL DEFAULT ''
		)`, m.ledgerName()))
	if err != nil {
		return fmt.Errorf("cannot create migration ledger")
	}
	return nil
}

func (m Migrator) applied(ctx context.Context) (map[int64]MigrationRecord, error) {
	var exists sql.NullString
	if err := m.executor().QueryRowContext(ctx, `SELECT to_regclass($1)::text`, "public."+m.ledgerName()).Scan(&exists); err != nil {
		return nil, fmt.Errorf("cannot inspect migration ledger")
	}
	if !exists.Valid {
		return map[int64]MigrationRecord{}, nil
	}
	rows, err := m.executor().QueryContext(ctx,
		fmt.Sprintf(`SELECT version, name, checksum, applied_at,origin,reviewed_by FROM %s ORDER BY version`, m.ledgerName()))
	if err != nil {
		return nil, fmt.Errorf("cannot read migration ledger")
	}
	defer rows.Close()
	applied := map[int64]MigrationRecord{}
	for rows.Next() {
		var record MigrationRecord
		if err := rows.Scan(&record.Version, &record.Name, &record.Checksum, &record.AppliedAt, &record.Origin, &record.ReviewedBy); err != nil {
			return nil, fmt.Errorf("cannot read migration ledger")
		}
		applied[record.Version] = record
	}
	return applied, rows.Err()
}

// Verify checks that already-applied migrations still match their files. A
// checksum change under an existing version means the schema and the code
// disagree, which must block the platform rather than be ignored.
func (m Migrator) Verify(ctx context.Context, files []MigrationFile) ([]string, []MigrationRecord, error) {
	if err := m.validate(); err != nil {
		return nil, nil, err
	}
	if err := validateMigrationFiles(files); err != nil {
		return nil, nil, err
	}
	applied, err := m.applied(ctx)
	if err != nil {
		return nil, nil, err
	}
	byVersion := map[int64]MigrationFile{}
	for _, file := range files {
		byVersion[file.Version] = file
	}
	var problems []string
	var matched []MigrationRecord
	for version, record := range applied {
		file, ok := byVersion[version]
		if !ok {
			problems = append(problems, fmt.Sprintf("migration %d (%s) is recorded but no longer exists", version, record.Name))
			continue
		}
		if file.Checksum != record.Checksum || file.Name != record.Name {
			problems = append(problems, fmt.Sprintf("migration %d (%s) checksum differs from the applied record", version, record.Name))
			continue
		}
		matched = append(matched, record)
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].Version < matched[j].Version })
	sort.Strings(problems)
	return problems, matched, nil
}

// Apply runs pending migrations while holding the owner's advisory lock. The lock
// is what stops two installers from interleaving schema changes; the ledger is
// what makes a repeated run a no-op instead of a second application.
func (m Migrator) Apply(ctx context.Context, files []MigrationFile) (*MigrationOutcome, error) {
	if err := validateMigrationFiles(files); err != nil {
		return nil, err
	}
	locked, release, err := m.locked(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	m = locked
	if err := m.ensureLedger(ctx); err != nil {
		return nil, err
	}

	problems, matched, err := m.Verify(ctx, files)
	if err != nil {
		return nil, err
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("migration verification failed: %s", strings.Join(problems, "; "))
	}
	fingerprintBefore, err := m.schemaFingerprint(ctx)
	if err != nil {
		return nil, err
	}
	appliedSet := map[int64]bool{}
	var highestApplied int64
	for _, record := range matched {
		appliedSet[record.Version] = true
		if record.Version > highestApplied {
			highestApplied = record.Version
		}
	}
	empty := sha256.Sum256(nil)
	if len(matched) == 0 && fingerprintBefore != hex.EncodeToString(empty[:]) {
		return nil, fmt.Errorf("existing application schema requires an explicitly reviewed baseline")
	}
	for _, file := range files {
		if !appliedSet[file.Version] && file.Version < highestApplied {
			return nil, fmt.Errorf("baseline or migration history has an unapplied earlier version; review before executing")
		}
	}
	outcome := &MigrationOutcome{Owner: m.Owner, Skipped: matched, SchemaBefore: fingerprintBefore}
	for _, file := range files {
		if appliedSet[file.Version] {
			continue
		}
		// Normalize the outer wrapper and commit DDL and its ledger row together;
		// even legacy BEGIN/COMMIT files cannot escape this transaction.
		record := MigrationRecord{Version: file.Version, Name: file.Name, Checksum: file.Checksum, AppliedAt: m.now(), Origin: "executed"}
		body, err := transactionalBody(file.SQL)
		if err != nil {
			return nil, fmt.Errorf("migration %d has unsupported transaction controls", file.Version)
		}
		tx, err := m.conn.BeginTx(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("cannot begin migration %d", file.Version)
		}
		if _, err := tx.ExecContext(ctx, body); err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("migration %d (%s) failed and was rolled back", file.Version, file.Name)
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(
			`INSERT INTO %s (version, name, checksum, applied_at) VALUES ($1, $2, $3, $4)`,
			m.ledgerName()), file.Version, file.Name, file.Checksum, record.AppliedAt); err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("migration %d (%s) could not be recorded", file.Version, file.Name)
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("migration %d (%s) could not be committed", file.Version, file.Name)
		}
		outcome.Applied = append(outcome.Applied, record)
	}
	fingerprintAfter, err := m.schemaFingerprint(ctx)
	if err != nil {
		return nil, err
	}
	outcome.SchemaAfter = fingerprintAfter
	return outcome, nil
}

// VerifySchemaForService reports whether the schema behind a credential is the
// one the platform expects. A mismatch must keep the service out of readiness
// rather than let it start against an incompatible schema.
func (m Migrator) VerifySchemaForService(ctx context.Context, files []MigrationFile) (bool, []string, error) {
	problems, matched, err := m.Verify(ctx, files)
	if err != nil {
		return false, nil, err
	}
	pending := 0
	appliedSet := map[int64]bool{}
	for _, record := range matched {
		appliedSet[record.Version] = true
	}
	for _, file := range files {
		if !appliedSet[file.Version] {
			pending++
		}
	}
	if pending > 0 {
		problems = append(problems, fmt.Sprintf("%d migrations are not applied", pending))
	}
	if len(problems) > 0 {
		return false, problems, nil
	}
	return true, nil, nil
}
