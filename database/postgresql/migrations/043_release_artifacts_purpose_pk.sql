-- Migration: 043_release_artifacts_purpose_pk
-- Description: Allow the same artifact to be referenced multiple times in one
-- release under different purposes (e.g. runtime + sbom + attestation) by
-- widening the primary key to include purpose.
-- Tiers: All
-- Dependencies: 034_release_artifacts

ALTER TABLE release_artifacts DROP CONSTRAINT IF EXISTS release_artifacts_pkey;

CREATE UNIQUE INDEX IF NOT EXISTS uq_release_artifacts_release_artifact_purpose
    ON release_artifacts (release_id, artifact_id, purpose);

ALTER TABLE release_artifacts
    ADD CONSTRAINT release_artifacts_pkey PRIMARY KEY USING INDEX uq_release_artifacts_release_artifact_purpose;
