package iam

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// RecordKeyManifest persists public metadata and append-only lifecycle facts.
// The manifest model has no field capable of carrying private key material.
func (s *IAMDBStore) RecordKeyManifest(ctx context.Context, manifest KeyManifest) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, manifest.Issuer); err != nil {
		return err
	}
	var storedGeneration uint64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version), 0) FROM signing_key_metadata WHERE issuer = $1`, manifest.Issuer).Scan(&storedGeneration); err != nil {
		return err
	}
	if storedGeneration > manifest.Generation {
		return errors.New("stored signing-key generation is newer than manifest")
	}
	for kid, entry := range manifest.Keys {
		if err := recordManifestKey(ctx, tx, manifest, kid, entry, storedGeneration); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func recordManifestKey(ctx context.Context, tx *sql.Tx, manifest KeyManifest, kid string, entry KeyManifestEntry, storedGeneration uint64) error {
	var id, oldStatus, oldRef string
	var version uint64
	var oldNotBefore, oldNotAfter time.Time
	err := tx.QueryRowContext(ctx, `
		SELECT id::text, status, version, verification_key_ref, not_before, not_after
		FROM signing_key_metadata WHERE issuer = $1 AND key_id = $2 FOR UPDATE`, manifest.Issuer, kid).
		Scan(&id, &oldStatus, &version, &oldRef, &oldNotBefore, &oldNotAfter)
	if errors.Is(err, sql.ErrNoRows) {
		if storedGeneration != 0 && manifest.Generation <= storedGeneration {
			return errors.New("key cannot be added by reusing a stored manifest generation")
		}
		err = tx.QueryRowContext(ctx, `
			INSERT INTO signing_key_metadata
				(issuer, key_id, algorithm, signing_provider, signing_key_handle, verification_key_ref,
				 status, version, not_before, not_after, activated_at, retiring_at, revoked_at)
			VALUES ($1, $2, 'ES256', 'manifest', $3, $4, $5, $6, $7, $8,
				CASE WHEN $5 = 'active' THEN NOW() END,
				CASE WHEN $5 = 'retiring' THEN NOW() END,
				CASE WHEN $5 = 'revoked' THEN NOW() END)
			RETURNING id::text`, manifest.Issuer, kid, fmt.Sprintf("manifest:%d:%s", manifest.Generation, kid),
			entry.PublicKeyPath, entry.Status, manifest.Generation, entry.NotBefore, entry.NotAfter).Scan(&id)
		if err != nil {
			return err
		}
		if err := appendKeyLifecycleEvent(ctx, tx, id, "created"); err != nil {
			return err
		}
		if entry.Status != KeyPending {
			return appendKeyLifecycleEvent(ctx, tx, id, lifecycleEvent(entry.Status))
		}
		return nil
	}
	if err != nil {
		return err
	}
	if version > manifest.Generation {
		return errors.New("stored signing-key generation is newer than manifest")
	}
	if oldRef != entry.PublicKeyPath || !oldNotBefore.Equal(entry.NotBefore) || !oldNotAfter.Equal(entry.NotAfter) {
		return errors.New("stored signing-key immutable metadata differs from manifest")
	}
	if !validKeyStatusTransition(KeyStatus(oldStatus), entry.Status) {
		return fmt.Errorf("stored signing key has forbidden status transition %s -> %s", oldStatus, entry.Status)
	}
	if version == manifest.Generation {
		if oldStatus != string(entry.Status) {
			return errors.New("stored signing-key generation has different metadata")
		}
		return nil
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE signing_key_metadata SET
			status = $2, version = $3, verification_key_ref = $4,
			activated_at = CASE WHEN $2 = 'active' AND status <> 'active' THEN NOW() ELSE activated_at END,
			retiring_at = CASE WHEN $2 = 'retiring' AND status <> 'retiring' THEN NOW() ELSE retiring_at END,
			revoked_at = CASE WHEN $2 = 'revoked' AND status <> 'revoked' THEN NOW() ELSE revoked_at END,
			updated_at = NOW()
		WHERE id = $1::uuid`, id, entry.Status, manifest.Generation, entry.PublicKeyPath)
	if err != nil {
		return err
	}
	if oldStatus != string(entry.Status) {
		return appendKeyLifecycleEvent(ctx, tx, id, lifecycleEvent(entry.Status))
	}
	return nil
}

func lifecycleEvent(status KeyStatus) string {
	switch status {
	case KeyActive:
		return "activated"
	case KeyRetiring:
		return "rotation_started"
	case KeyRevoked:
		return "revoked"
	case KeyExpired:
		return "expired"
	default:
		return "created"
	}
}

func appendKeyLifecycleEvent(ctx context.Context, tx *sql.Tx, id, event string) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO signing_key_lifecycle_events (signing_key_id, event_type, correlation_id, reason)
		VALUES ($1::uuid, $2, gen_random_uuid(), 'key manifest reload')`, id, event)
	return err
}
