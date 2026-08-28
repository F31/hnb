-- Migration: 042_artifact_storage_profile_and_updated_at
-- Description: Add storage_profile_id (already present in production from 035) and
-- updated_at to artifacts to align with other core tables.
-- Tiers: All
-- Dependencies: 035_artifact_storage_profile, 011_app_market_engine

ALTER TABLE artifacts
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP WITH TIME ZONE;

UPDATE artifacts
    SET updated_at = created_at
    WHERE updated_at IS NULL;

ALTER TABLE artifacts
    ALTER COLUMN updated_at SET NOT NULL,
    ALTER COLUMN updated_at SET DEFAULT NOW();
