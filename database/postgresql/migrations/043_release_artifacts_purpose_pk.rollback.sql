-- Rollback: 043_release_artifacts_purpose_pk
ALTER TABLE release_artifacts DROP CONSTRAINT IF EXISTS release_artifacts_pkey;

CREATE UNIQUE INDEX IF NOT EXISTS uq_release_artifacts_release_artifact
    ON release_artifacts (release_id, artifact_id);

ALTER TABLE release_artifacts
    ADD CONSTRAINT release_artifacts_pkey PRIMARY KEY USING INDEX uq_release_artifacts_release_artifact;
