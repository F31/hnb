package db

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
)

const MigrationBaselineSchema = "migration-baseline/v1"

type BaselineEntry struct {
	Version    int64  `yaml:"version" json:"version"`
	Name       string `yaml:"name" json:"name"`
	Checksum   string `yaml:"checksum" json:"checksum"`
	LegacyName string `yaml:"legacyName,omitempty" json:"legacyName,omitempty"`
}

// A baseline is an explicit review assertion for an existing database, not an
// inference that every SQL file in a directory has already run. Only its listed
// entries are adopted, and the database/schema snapshot must still match.
type MigrationBaseline struct {
	SchemaVersion     string          `yaml:"schemaVersion" json:"schemaVersion"`
	Owner             MigrationOwner  `yaml:"owner" json:"owner"`
	Database          string          `yaml:"database" json:"database"`
	SchemaFingerprint string          `yaml:"schemaFingerprint" json:"schemaFingerprint"`
	ReviewedBy        string          `yaml:"reviewedBy" json:"reviewedBy"`
	Entries           []BaselineEntry `yaml:"entries" json:"entries"`
}

func DecodeMigrationBaseline(data []byte) (*MigrationBaseline, error) {
	var manifest MigrationBaseline
	// Strict JSON decoding: unknown fields are rejected so a typo cannot be
	// silently ignored by the adoption review.
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("baseline manifest must be strict JSON: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("baseline manifest must not contain additional documents")
	}
	return &manifest, nil
}

func (m Migrator) InspectSchema(ctx context.Context) (string, string, error) {
	if err := m.validate(); err != nil {
		return "", "", err
	}
	var name string
	if err := m.executor().QueryRowContext(ctx, `SELECT current_database()`).Scan(&name); err != nil {
		return "", "", fmt.Errorf("cannot inspect database identity")
	}
	digest, err := m.schemaFingerprint(ctx)
	return name, digest, err
}

// Baseline adopts approved identities without executing their SQL. Record writes
// are atomic and share the same owner session lock as Apply. Existing rows must
// match exactly; discrepancies are never overwritten.
func (m Migrator) Baseline(ctx context.Context, files []MigrationFile, manifest *MigrationBaseline) error {
	if err := validateMigrationFiles(files); err != nil {
		return err
	}
	if manifest == nil || manifest.SchemaVersion != MigrationBaselineSchema || manifest.Owner != m.Owner || manifest.Database == "" || len(manifest.SchemaFingerprint) != 64 || manifest.ReviewedBy == "" || len(manifest.Entries) == 0 {
		return fmt.Errorf("explicit reviewed database-bound baseline is required")
	}
	byVersion := map[int64]MigrationFile{}
	for _, f := range files {
		byVersion[f.Version] = f
	}
	seen := map[int64]bool{}
	for _, entry := range manifest.Entries {
		f, ok := byVersion[entry.Version]
		if !ok || seen[entry.Version] || entry.Name != f.Name || entry.Checksum != f.Checksum {
			return fmt.Errorf("baseline identities do not match the current migration set")
		}
		seen[entry.Version] = true
	}
	locked, release, err := m.locked(ctx)
	if err != nil {
		return err
	}
	defer release()
	m = locked
	dbName, fingerprint, err := m.InspectSchema(ctx)
	if err != nil {
		return err
	}
	if dbName != manifest.Database || fingerprint != manifest.SchemaFingerprint {
		return fmt.Errorf("baseline database or schema snapshot changed; review again")
	}
	tx, err := m.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("cannot begin baseline")
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (version bigint PRIMARY KEY,name text NOT NULL,checksum char(64) NOT NULL,applied_at timestamptz NOT NULL,origin text NOT NULL DEFAULT 'executed',reviewed_by text NOT NULL DEFAULT '',legacy_name text NOT NULL DEFAULT '',baseline_schema_fingerprint text NOT NULL DEFAULT '')`, m.ledgerName()))
	if err != nil {
		return fmt.Errorf("cannot create baseline ledger")
	}
	for _, entry := range manifest.Entries {
		var name, checksum string
		err := tx.QueryRowContext(ctx, fmt.Sprintf(`SELECT name,checksum FROM %s WHERE version=$1`, m.ledgerName()), entry.Version).Scan(&name, &checksum)
		if err == nil {
			if name != entry.Name || checksum != entry.Checksum {
				return fmt.Errorf("existing baseline record conflicts")
			}
			continue
		}
		if err != sql.ErrNoRows {
			return fmt.Errorf("cannot read baseline record")
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`INSERT INTO %s (version,name,checksum,applied_at,origin,reviewed_by,legacy_name,baseline_schema_fingerprint) VALUES ($1,$2,$3,$4,'baseline',$5,$6,$7)`, m.ledgerName()), entry.Version, entry.Name, entry.Checksum, m.now(), manifest.ReviewedBy, entry.LegacyName, manifest.SchemaFingerprint); err != nil {
			return fmt.Errorf("cannot write baseline record")
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("cannot commit baseline")
	}
	return nil
}
