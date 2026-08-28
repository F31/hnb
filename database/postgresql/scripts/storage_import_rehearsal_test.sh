#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
container="hnb-storage-import-rehearsal-$$"

cleanup() {
    docker rm -f "$container" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

if ! command -v docker >/dev/null 2>&1 || ! docker info >/dev/null 2>&1; then
    echo "Docker is required for the isolated PostgreSQL 16 rehearsal." >&2
    exit 77
fi

docker run --rm --detach --name "$container" \
    --env POSTGRES_PASSWORD=test123 --publish 127.0.0.1::5432 postgres:16 >/dev/null
port="$(docker port "$container" 5432/tcp | while IFS= read -r line; do printf '%s' "${line##*:}"; break; done)"
url="postgresql://postgres:test123@127.0.0.1:${port}/postgres"
for _ in {1..60}; do
    if psql -X "$url" -c 'SELECT 1' >/dev/null 2>&1; then break; fi
    sleep 1
done

DATABASE_URL="$url" bash "$script_dir/migrate.sh" >/dev/null
psql -X --set=ON_ERROR_STOP=1 "$url" <<'SQL' >/dev/null
INSERT INTO tenants (id, name, display_name) VALUES
 ('import-a', 'import-a', 'Import A'), ('import-b', 'import-b', 'Import B');
INSERT INTO runtime_targets (id, tenant_id, name, target_type) VALUES
 ('7a000000-0000-0000-0000-000000000001', 'import-a', 'target-a', 'kubernetes'),
 ('7b000000-0000-0000-0000-000000000001', 'import-b', 'target-b', 'kubernetes');
INSERT INTO runtime_target_storage_inventory (
 tenant_id, target_id, resource_kind, resource_uid, resource_version, name,
 driver_name, source, observed_at, stale_after, observation_source,
 observation_source_id, observation_generation, observation_revision, attributes
) VALUES
 ('import-a', '7a000000-0000-0000-0000-000000000001', 'StorageClass', 'uid-fast', '41', 'fast',
  'csi.example.io', 'kubernetes.storage.k8s.io/v1', now(), now() + interval '1 hour', 'agent',
  'agent-a', 3, 7, '{"isDefault":true}'),
 ('import-a', '7a000000-0000-0000-0000-000000000001', 'StorageClass', 'uid-archive', '9', 'archive',
  'nfs.example.io', 'kubernetes.storage.k8s.io/v1', now(), now() + interval '1 hour', 'agent',
  'agent-a', 3, 7, '{"isDefault":false}'),
 ('import-b', '7b000000-0000-0000-0000-000000000001', 'StorageClass', 'uid-private', '12', 'private',
  'csi.private.io', 'kubernetes.storage.k8s.io/v1', now(), now() + interval '1 hour', 'agent',
  'agent-b', 2, 4, '{}');
SQL

inventory_before="$(psql -X -Aqt "$url" -c "SELECT md5(string_agg(row_to_json(i)::text, '|' ORDER BY tenant_id,target_id,resource_uid)) FROM runtime_target_storage_inventory i")"

dry_run_output="$(DATABASE_URL="$url" bash "$script_dir/import-observed-storage-classes.sh" \
    --tenant import-a --target 7a000000-0000-0000-0000-000000000001)"
[[ "$dry_run_output" == *"/container/storage?target="*"&cluster="*"&offering="*"&storageClass="* ]]
[[ "$(psql -X -Aqt "$url" -c 'SELECT count(*) FROM workload_storage_offerings')" == "0" ]]
[[ "$(psql -X -Aqt "$url" -c 'SELECT count(*) FROM storage_class_bindings')" == "0" ]]
echo "PASS dry-run rollback: 0 offerings, 0 bindings"

DATABASE_URL="$url" bash "$script_dir/import-observed-storage-classes.sh" \
    --tenant import-a --target 7a000000-0000-0000-0000-000000000001 --apply >/dev/null
first_digest="$(psql -X -Aqt "$url" -c "SELECT md5(string_agg(row_to_json(x)::text, '|' ORDER BY kind,id)) FROM (SELECT 'offering' kind,id,version,created_at,updated_at FROM workload_storage_offerings UNION ALL SELECT 'binding',id,version,created_at,updated_at FROM storage_class_bindings) x")"
DATABASE_URL="$url" bash "$script_dir/import-observed-storage-classes.sh" \
    --tenant import-a --target 7a000000-0000-0000-0000-000000000001 --apply >/dev/null
second_digest="$(psql -X -Aqt "$url" -c "SELECT md5(string_agg(row_to_json(x)::text, '|' ORDER BY kind,id)) FROM (SELECT 'offering' kind,id,version,created_at,updated_at FROM workload_storage_offerings UNION ALL SELECT 'binding',id,version,created_at,updated_at FROM storage_class_bindings) x")"
[[ "$first_digest" == "$second_digest" ]]
[[ "$(psql -X -Aqt "$url" -c 'SELECT count(*) FROM workload_storage_offerings')" == "2" ]]
[[ "$(psql -X -Aqt "$url" -c 'SELECT count(*) FROM storage_class_bindings')" == "2" ]]
[[ "$(psql -X -Aqt "$url" -c "SELECT bool_and(storage_class_uid IN ('uid-fast','uid-archive') AND storage_class_resource_version IN ('41','9') AND version=1) FROM storage_class_bindings")" == "t" ]]
echo "PASS duplicate import: stable digest, 2 offerings, 2 bindings, versions and Kubernetes identities preserved"

if DATABASE_URL="$url" bash "$script_dir/import-observed-storage-classes.sh" \
    --tenant import-a --target 7b000000-0000-0000-0000-000000000001 --apply >/dev/null 2>&1; then
    echo "cross-tenant import unexpectedly succeeded" >&2
    exit 1
fi
[[ "$(psql -X -Aqt "$url" -c "SELECT count(*) FROM storage_class_bindings WHERE tenant_id='import-b'")" == "0" ]]
echo "PASS cross-tenant rejection: tenant B desired rows remain 0"

context="$(DATABASE_URL="$url" bash "$script_dir/import-observed-storage-classes.sh" \
    --tenant import-a --target 7a000000-0000-0000-0000-000000000001)"
[[ "$context" == *"cluster=7a000000-0000-0000-0000-000000000001"* ]]
[[ "$context" == *"target=7a000000-0000-0000-0000-000000000001"* ]]
[[ "$context" == *"&offering="*"&storageClass=fast"* ]]
echo "PASS query context: target, cluster, offering and storageClass filters emitted"

inventory_after="$(psql -X -Aqt "$url" -c "SELECT md5(string_agg(row_to_json(i)::text, '|' ORDER BY tenant_id,target_id,resource_uid)) FROM runtime_target_storage_inventory i")"
[[ "$inventory_before" == "$inventory_after" ]]
if grep -Eiq 'kubectl|kubernetes[^[:space:]]* (delete|patch|apply)|DELETE[[:space:]]+FROM[[:space:]]+runtime_target_storage_inventory|UPDATE[[:space:]]+runtime_target_storage_inventory' \
    "$script_dir/import-observed-storage-classes.sh" "$script_dir/import-observed-storage-classes.sql"; then
    echo "target mutation surface detected" >&2
    exit 1
fi
echo "PASS target safety: observed inventory digest unchanged and no Kubernetes mutation/delete path"
echo "PASS PostgreSQL: $(psql -X -Aqt "$url" -c 'SHOW server_version' | tr -d '[:space:]')"
