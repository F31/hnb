-- Migration: 034_release_artifacts
-- Description: Normalize Release to ArtifactDescriptor references for digest-pinned execution.
-- Tiers: All
-- Dependencies: 033_artifact_descriptors

CREATE TABLE IF NOT EXISTS release_artifacts (
    release_id UUID NOT NULL REFERENCES releases(id) ON DELETE CASCADE,
    artifact_id UUID NOT NULL REFERENCES artifacts(id) ON DELETE RESTRICT,
    purpose TEXT NOT NULL DEFAULT 'runtime',
    position INTEGER NOT NULL DEFAULT 0,
    digest TEXT NOT NULL CHECK (digest ~ '^sha256:[0-9a-f]{64}$'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (release_id, artifact_id)
);

CREATE INDEX IF NOT EXISTS idx_release_artifacts_artifact_id ON release_artifacts(artifact_id);
CREATE INDEX IF NOT EXISTS idx_release_artifacts_digest ON release_artifacts(digest);

-- Best-effort reconciliation for legacy manifests that already contain resolvable verified digests.
INSERT INTO release_artifacts (release_id, artifact_id, purpose, position, digest)
SELECT r.id, a.id, COALESCE(NULLIF(ref.value->>'purpose', ''), 'runtime'), ref.ordinality::INTEGER - 1, a.digest
FROM releases r
JOIN products p ON p.id = r.product_id
JOIN publishers pub ON pub.id = p.publisher_id
JOIN LATERAL jsonb_array_elements(COALESCE(r.manifest->'artifacts', '[]'::jsonb)) WITH ORDINALITY AS ref(value, ordinality) ON true
JOIN artifacts a ON a.tenant_id = pub.tenant_id
    AND a.digest = ref.value->>'digest'
    AND a.verification_status = 'verified'
    AND a.lifecycle_state = 'active'
WHERE ref.value->>'digest' ~ '^sha256:[0-9a-f]{64}$'
-- Migration 043 widens the key with purpose; target-less conflict handling is
-- compatible with both the original and current key shapes during replay.
ON CONFLICT DO NOTHING;
