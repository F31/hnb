-- 106: immutable application delivery revisions, scoped to one tenant/target release.
-- A binding row is append-only per revision; the current view selects the highest
-- revision per (tenant, target, namespace, release). PVC and ingress references
-- are recorded by name only; credentials never enter this table.
BEGIN;

CREATE TABLE IF NOT EXISTS application_delivery_bindings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    target_id UUID NOT NULL,
    namespace TEXT NOT NULL CHECK (length(namespace) BETWEEN 1 AND 63),
    release_id TEXT NOT NULL CHECK (length(release_id) BETWEEN 1 AND 256),
    release_version TEXT NOT NULL CHECK (length(release_version) BETWEEN 1 AND 128),
    revision BIGINT NOT NULL CHECK (revision > 0),
    resource_uid TEXT NOT NULL CHECK (length(resource_uid) BETWEEN 1 AND 256),
    pvc_name TEXT CHECK (pvc_name IS NULL OR length(pvc_name) BETWEEN 1 AND 253),
    ingress_name TEXT CHECK (ingress_name IS NULL OR length(ingress_name) BETWEEN 1 AND 253),
    operation_id UUID,
    observation_generation BIGINT CHECK (observation_generation IS NULL OR observation_generation >= 0),
    observation_revision BIGINT CHECK (observation_revision IS NULL OR observation_revision >= 0),
    status TEXT NOT NULL CHECK (status IN ('Planned','Deploying','Ready','Failed','FailedVerification','Deleting','Deleted','Retained')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT application_delivery_target_tenant_fk
        FOREIGN KEY (target_id, tenant_id) REFERENCES runtime_targets(id, tenant_id) ON DELETE RESTRICT,
    CONSTRAINT application_delivery_revision_unique
        UNIQUE (tenant_id, target_id, namespace, release_id, revision)
);

CREATE INDEX IF NOT EXISTS idx_application_delivery_tenant_target
    ON application_delivery_bindings(tenant_id, target_id, created_at DESC, revision DESC);
CREATE INDEX IF NOT EXISTS idx_application_delivery_tenant_release
    ON application_delivery_bindings(tenant_id, release_id, revision DESC);

COMMIT;
