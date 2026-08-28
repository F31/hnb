\set ON_ERROR_STOP on

BEGIN;

CREATE TEMP TABLE storage_import_request ON COMMIT DROP AS
SELECT :'tenant_id'::text AS tenant_id, :'target_id'::uuid AS target_id;

DO $$
DECLARE
    request storage_import_request%ROWTYPE;
BEGIN
    SELECT * INTO STRICT request FROM storage_import_request;
    IF NOT EXISTS (
        SELECT 1 FROM runtime_targets
        WHERE id = request.target_id AND tenant_id = request.tenant_id
    ) THEN
        RAISE EXCEPTION 'target % is not owned by tenant %', request.target_id, request.tenant_id
            USING ERRCODE = '42501';
    END IF;
END
$$;

CREATE TEMP TABLE storage_import_candidates ON COMMIT DROP AS
SELECT
    inventory.tenant_id,
    inventory.target_id,
    inventory.resource_uid,
    inventory.resource_version,
    inventory.name,
    inventory.observed_at,
    CASE WHEN inventory.stale_after > now() THEN 'Fresh' ELSE 'Stale' END AS freshness,
    COALESCE((inventory.attributes ->> 'isDefault')::boolean, false) AS is_default,
    md5('storage-import/offering/' || inventory.tenant_id || '/' || inventory.target_id || '/' || inventory.resource_uid)::uuid AS offering_id,
    md5('storage-import/binding/' || inventory.tenant_id || '/' || inventory.target_id || '/' || inventory.resource_uid)::uuid AS binding_id,
    left('Imported ' || inventory.name || ' [' || inventory.target_id || ']', 256) AS offering_name
FROM runtime_target_storage_inventory inventory
JOIN storage_import_request request
  ON request.tenant_id = inventory.tenant_id AND request.target_id = inventory.target_id
WHERE inventory.resource_kind = 'StorageClass'
  AND inventory.deleted_at IS NULL;

-- StorageClass does not report workload mode/access modes. Imported offerings
-- remain review-required and deliberately claim no advanced capabilities.
INSERT INTO workload_storage_offerings (
    id, tenant_id, name, description, service_mode, access_modes,
    volume_expansion, snapshots, clones, protection_class
)
SELECT
    offering_id, tenant_id, offering_name,
    'Imported from observed StorageClass; workload semantics require operator review.',
    'Block', '["ReadWriteOnce"]'::jsonb,
    'Unknown', 'Unknown', 'Unknown', 'import-review-required'
FROM storage_import_candidates
ON CONFLICT DO NOTHING;

INSERT INTO storage_class_bindings (
    id, tenant_id, offering_id, offering_version, target_id,
    storage_class_name, storage_class_uid, storage_class_resource_version,
    sync_state, is_default, source, observed_at, freshness, conditions,
    observation_generation, observation_revision
)
SELECT
    candidate.binding_id, candidate.tenant_id, candidate.offering_id, offering.version,
    candidate.target_id, candidate.name, candidate.resource_uid, candidate.resource_version,
    'Imported', candidate.is_default, 'runtime_target_storage_inventory',
    candidate.observed_at, candidate.freshness,
    '[{"type":"ReviewRequired","status":"True","reason":"StorageClassDoesNotDeclareWorkloadSemantics"}]'::jsonb,
    inventory.observation_generation, inventory.observation_revision
FROM storage_import_candidates candidate
JOIN workload_storage_offerings offering
  ON offering.id = candidate.offering_id AND offering.tenant_id = candidate.tenant_id
JOIN runtime_target_storage_inventory inventory
  ON inventory.tenant_id = candidate.tenant_id
 AND inventory.target_id = candidate.target_id
 AND inventory.resource_kind = 'StorageClass'
 AND inventory.resource_uid = candidate.resource_uid
ON CONFLICT DO NOTHING;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM storage_import_candidates candidate
        LEFT JOIN workload_storage_offerings offering
          ON offering.id = candidate.offering_id AND offering.tenant_id = candidate.tenant_id
        LEFT JOIN storage_class_bindings binding
          ON binding.id = candidate.binding_id
         AND binding.tenant_id = candidate.tenant_id
         AND binding.offering_id = candidate.offering_id
         AND binding.target_id = candidate.target_id
         AND binding.storage_class_uid = candidate.resource_uid
         AND binding.storage_class_resource_version = candidate.resource_version
        WHERE offering.id IS NULL OR binding.id IS NULL
    ) THEN
        RAISE EXCEPTION 'import conflict: existing desired state does not match observed tenant/target/UID/resourceVersion';
    END IF;
END
$$;

SELECT
    candidate.target_id AS "targetId",
    candidate.offering_id AS "offeringId",
    candidate.binding_id AS "bindingId",
    candidate.name AS "storageClassName",
    candidate.resource_uid AS "storageClassUid",
    candidate.resource_version AS "storageClassResourceVersion",
    '/container/storage?target=' || candidate.target_id ||
        '&cluster=' || candidate.target_id ||
        '&offering=' || candidate.offering_id ||
        '&storageClass=' || replace(candidate.name, ' ', '%20') AS "queryContext"
FROM storage_import_candidates candidate
ORDER BY candidate.name, candidate.resource_uid;

\if :dry_run
ROLLBACK;
\else
COMMIT;
\endif
