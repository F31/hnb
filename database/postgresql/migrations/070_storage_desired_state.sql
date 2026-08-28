-- 070: storage_desired_state
-- Description: Add tenant-scoped authoritative storage backend, offering, and binding records.
-- Dependencies: 069_storage_inventory_projection

BEGIN;

CREATE TABLE IF NOT EXISTS storage_backends (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL,
    provider_type TEXT NOT NULL CHECK (length(provider_type) BETWEEN 1 AND 128),
    backend_id TEXT NOT NULL CHECK (length(backend_id) BETWEEN 1 AND 256),
    display_name TEXT NOT NULL CHECK (length(display_name) BETWEEN 1 AND 256),
    description TEXT NOT NULL DEFAULT '' CHECK (length(description) <= 2048),
    secret_provider TEXT,
    secret_scope TEXT,
    secret_name TEXT,
    secret_version TEXT,
    version BIGINT NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (id, tenant_id),
    UNIQUE (tenant_id, provider_type, backend_id),
    FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE RESTRICT,
    CHECK (
        (secret_provider IS NULL AND secret_scope IS NULL AND secret_name IS NULL AND secret_version IS NULL)
        OR
        (length(secret_provider) >= 1 AND length(secret_scope) >= 1 AND length(secret_name) >= 1)
    )
);

CREATE TABLE IF NOT EXISTS workload_storage_offerings (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL,
    backend_id UUID,
    name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 256),
    description TEXT NOT NULL DEFAULT '' CHECK (length(description) <= 2048),
    service_mode TEXT NOT NULL CHECK (service_mode IN ('Block', 'File')),
    access_modes JSONB NOT NULL CHECK (jsonb_typeof(access_modes) = 'array' AND jsonb_array_length(access_modes) BETWEEN 1 AND 4),
    volume_expansion TEXT NOT NULL CHECK (volume_expansion IN ('Supported', 'Unsupported', 'Unknown')),
    snapshots TEXT NOT NULL CHECK (snapshots IN ('Supported', 'Unsupported', 'Unknown')),
    clones TEXT NOT NULL CHECK (clones IN ('Supported', 'Unsupported', 'Unknown')),
    topology JSONB NOT NULL DEFAULT '{}',
    protection_class TEXT NOT NULL CHECK (length(protection_class) BETWEEN 1 AND 128),
    version BIGINT NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (id, tenant_id),
    UNIQUE (tenant_id, name),
    FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE RESTRICT,
    FOREIGN KEY (backend_id, tenant_id) REFERENCES storage_backends(id, tenant_id) ON DELETE RESTRICT,
    CHECK (jsonb_typeof(topology) = 'object')
);

CREATE TABLE IF NOT EXISTS storage_class_bindings (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL,
    offering_id UUID NOT NULL,
    offering_version BIGINT NOT NULL CHECK (offering_version >= 1),
    target_id UUID NOT NULL,
    storage_class_name TEXT NOT NULL CHECK (length(storage_class_name) BETWEEN 1 AND 253),
    storage_class_uid TEXT NOT NULL CHECK (length(storage_class_uid) BETWEEN 1 AND 128),
    storage_class_resource_version TEXT NOT NULL CHECK (length(storage_class_resource_version) BETWEEN 1 AND 128),
    sync_state TEXT NOT NULL CHECK (sync_state IN ('Discovered', 'Imported', 'Active', 'Drifted', 'Rejected', 'Retired')),
    is_default BOOLEAN NOT NULL DEFAULT false,
    source TEXT NOT NULL CHECK (length(source) BETWEEN 1 AND 256),
    observed_at TIMESTAMPTZ NOT NULL,
    freshness TEXT NOT NULL CHECK (freshness IN ('Fresh', 'Stale', 'Unknown')),
    topology JSONB NOT NULL DEFAULT '{}',
    version BIGINT NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (id),
    UNIQUE (id, tenant_id),
    UNIQUE (tenant_id, offering_id, target_id, storage_class_uid),
    FOREIGN KEY (offering_id, tenant_id) REFERENCES workload_storage_offerings(id, tenant_id) ON DELETE RESTRICT,
    FOREIGN KEY (target_id, tenant_id) REFERENCES runtime_targets(id, tenant_id) ON DELETE RESTRICT,
    CHECK (jsonb_typeof(topology) = 'object')
);

CREATE INDEX IF NOT EXISTS idx_storage_backends_tenant_name
    ON storage_backends(tenant_id, display_name, id);
CREATE INDEX IF NOT EXISTS idx_storage_offerings_tenant_name
    ON workload_storage_offerings(tenant_id, name, id);
CREATE INDEX IF NOT EXISTS idx_storage_bindings_offering
    ON storage_class_bindings(tenant_id, offering_id, target_id, storage_class_name, id);

COMMIT;
