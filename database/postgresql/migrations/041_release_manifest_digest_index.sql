-- Migration: 041_release_manifest_digest_index
-- Description: Allow identical manifests across independently versioned releases.
-- Tiers: All
-- Dependencies: 011_app_market_engine

DROP INDEX IF EXISTS idx_releases_manifest_digest;
CREATE INDEX IF NOT EXISTS idx_releases_manifest_digest ON releases(manifest_digest);
