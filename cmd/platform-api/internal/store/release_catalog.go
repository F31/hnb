package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ErrFixedReleaseConflict means the same release/version/role is already
// registered with a different content digest. The fixed release is never
// silently replaced.
var ErrFixedReleaseConflict = errors.New("fixed release already registered with a different content digest")

// ReleaseCatalogEntry is one validated release import for an install target
// role, bound to the installer release lock and offline bundle digests.
type ReleaseCatalogEntry struct {
	ID             string    `json:"id"`
	ReleaseID      string    `json:"releaseId"`
	ReleaseVersion string    `json:"releaseVersion"`
	Role           string    `json:"role"`
	BundleID       string    `json:"bundleId"`
	LockDigest     string    `json:"lockDigest"`
	ContentDigest  string    `json:"contentDigest"`
	ImportedBy     string    `json:"importedBy"`
	RegisteredAt   time.Time `json:"registeredAt"`
}

// ReleaseCatalogStore is the owning-plane data-access contract for the unified
// release material directory.
type ReleaseCatalogStore interface {
	// RegisterReleaseMaterial records a validated closure for one target role.
	// It returns created=true on first registration, created=false when the same
	// bundle identity is already present (idempotent), and ErrFixedReleaseConflict
	// when the fixed release content would change.
	RegisterReleaseMaterial(ctx context.Context, entry ReleaseCatalogEntry) (*ReleaseCatalogEntry, bool, error)
	ListReleaseMaterials(ctx context.Context) ([]ReleaseCatalogEntry, error)
}

func (s *PGStore) RegisterReleaseMaterial(ctx context.Context, entry ReleaseCatalogEntry) (*ReleaseCatalogEntry, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	var existing ReleaseCatalogEntry
	err = tx.QueryRowContext(ctx, `
		SELECT id, release_id, release_version, role, bundle_id, lock_digest, content_digest, imported_by, registered_at
		FROM release_catalog_entries
		WHERE release_id = $1 AND release_version = $2 AND role = $3
		FOR UPDATE`,
		entry.ReleaseID, entry.ReleaseVersion, entry.Role,
	).Scan(&existing.ID, &existing.ReleaseID, &existing.ReleaseVersion, &existing.Role,
		&existing.BundleID, &existing.LockDigest, &existing.ContentDigest, &existing.ImportedBy, &existing.RegisteredAt)
	switch {
	case err == nil:
		if existing.LockDigest != entry.LockDigest || existing.ContentDigest != entry.ContentDigest {
			return nil, false, ErrFixedReleaseConflict
		}
		return &existing, false, nil
	case errors.Is(err, sql.ErrNoRows):
		var id string
		err = tx.QueryRowContext(ctx, `
			INSERT INTO release_catalog_entries (release_id, release_version, role, bundle_id, lock_digest, content_digest, imported_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id, registered_at`,
			entry.ReleaseID, entry.ReleaseVersion, entry.Role, entry.BundleID,
			entry.LockDigest, entry.ContentDigest, entry.ImportedBy,
		).Scan(&id, &entry.RegisteredAt)
		if err != nil {
			return nil, false, err
		}
		entry.ID = id
		if err := tx.Commit(); err != nil {
			return nil, false, err
		}
		return &entry, true, nil
	default:
		return nil, false, err
	}
}

func (s *PGStore) ListReleaseMaterials(ctx context.Context) ([]ReleaseCatalogEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, release_id, release_version, role, bundle_id, lock_digest, content_digest, imported_by, registered_at
		FROM release_catalog_entries
		ORDER BY registered_at, release_id, release_version, role`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReleaseCatalogEntry
	for rows.Next() {
		var e ReleaseCatalogEntry
		if err := rows.Scan(&e.ID, &e.ReleaseID, &e.ReleaseVersion, &e.Role, &e.BundleID,
			&e.LockDigest, &e.ContentDigest, &e.ImportedBy, &e.RegisteredAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
