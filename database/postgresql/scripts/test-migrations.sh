#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
migrate="$script_dir/migrate.sh"
container="hnb-migration-test-$$"

static_checks() {
    bash -n "$migrate" "$0"
    [[ -f "$script_dir/../migrations/026_signing_key_metadata.sql" ]]
    [[ -f "$script_dir/../rollbacks/026_signing_key_metadata.rollback.sql" ]]
    [[ -f "$script_dir/../migrations/027_trusted_identity_tokens.sql" ]]
    [[ -f "$script_dir/../rollbacks/027_trusted_identity_tokens.rollback.sql" ]]
    [[ -f "$script_dir/../migrations/070_storage_desired_state.sql" ]]
    [[ -f "$script_dir/../migrations/070_storage_desired_state.rollback.sql" ]]
	[[ -f "$script_dir/../migrations/072_storage_binding_drift.sql" ]]
	[[ -f "$script_dir/../migrations/072_storage_binding_drift.rollback.sql" ]]
	[[ -f "$script_dir/../migrations/074_storage_volume_semantics_boundary.sql" ]]
	[[ -f "$script_dir/../migrations/074_storage_volume_semantics_boundary.rollback.sql" ]]
	[[ -f "$script_dir/../migrations/076_storage_alert_rules.sql" ]]
	[[ -f "$script_dir/../migrations/076_storage_alert_rules.rollback.sql" ]]
	[[ -f "$script_dir/../migrations/077_storage_navigation_cutover.sql" ]]
	[[ -f "$script_dir/../migrations/077_storage_navigation_cutover.rollback.sql" ]]
	[[ -f "$script_dir/storage_desired_state_test.sql" ]]
	[[ -f "$script_dir/storage_alert_rules_test.sql" ]]
	[[ -f "$script_dir/storage_navigation_test.sql" ]]
	[[ -f "$script_dir/resource_cluster_navigation_test.sql" ]]
    echo "Static migration checks passed; PostgreSQL verification was not run." >&2
}

if ! command -v docker >/dev/null 2>&1 || ! docker info >/dev/null 2>&1; then
    static_checks
    exit 77
fi

cleanup() {
    docker rm -f "$container" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

docker run --rm --detach --name "$container" \
    --env POSTGRES_USER=postgres \
    --env POSTGRES_PASSWORD=test123 \
    --publish 127.0.0.1::5432 \
    postgres:16 >/dev/null

port_lines="$(docker port "$container" 5432/tcp)"
port=""
while IFS= read -r line; do
    port="${line##*:}"
    break
done <<< "$port_lines"
if [[ -z "$port" ]]; then
    echo "could not determine temporary PostgreSQL port" >&2
    exit 1
fi

admin_url="postgresql://postgres:test123@127.0.0.1:${port}/postgres"
for _ in {1..60}; do
    if psql -X -v ON_ERROR_STOP=1 "$admin_url" -c 'SELECT 1' >/dev/null 2>&1; then
        break
    fi
    sleep 1
done
psql -X -v ON_ERROR_STOP=1 "$admin_url" -c 'SELECT version()' >/dev/null

create_database() {
    psql -X -v ON_ERROR_STOP=1 "$admin_url" -c "CREATE DATABASE $1" >/dev/null
}

database_url() {
    printf 'postgresql://postgres:test123@127.0.0.1:%s/%s' "$port" "$1"
}

run_forward() {
    local database="$1"
    local from="$2"
    local to="$3"
    PGOPTIONS='-c client_min_messages=warning' DATABASE_URL="$(database_url "$database")" \
        MIGRATION_FROM="$from" MIGRATION_TO="$to" \
        bash "$migrate" >/dev/null
}

expect_failure() {
    local database="$1"
    local statement="$2"
    if psql -X -v ON_ERROR_STOP=1 "$(database_url "$database")" -c "$statement" >/dev/null 2>&1; then
        echo "expected statement to fail: $statement" >&2
        exit 1
    fi
}

echo "[1/6] empty database forward and idempotent rerun"
create_database hnb_empty
run_forward hnb_empty 001 999
empty_url="$(database_url hnb_empty)"
psql -X -v ON_ERROR_STOP=1 "$empty_url" >/dev/null <<'SQL'
INSERT INTO tenants (id, name, display_name)
VALUES ('replay-tenant', 'replay-tenant', 'Replay Tenant');
INSERT INTO workspaces (id, tenant_id, name)
VALUES ('01000000-0000-0000-0000-000000000001', 'replay-tenant', 'replay-workspace');
INSERT INTO namespaces (id, workspace_id, tenant_id, name)
VALUES ('replay-namespace', '01000000-0000-0000-0000-000000000001', 'replay-tenant', 'replay-namespace');
INSERT INTO identity_subjects (id, issuer, external_subject, subject_type)
VALUES ('02000000-0000-0000-0000-000000000001', 'https://replay.example', 'replay-subject', 'user');
INSERT INTO tenant_memberships (tenant_id, subject_id)
VALUES ('replay-tenant', '02000000-0000-0000-0000-000000000001');
INSERT INTO scoped_roles (id, tenant_id, name)
VALUES ('03000000-0000-0000-0000-000000000001', 'replay-tenant', 'replay-role');
INSERT INTO scoped_role_bindings (
    id, tenant_id, subject_id, role_id, scope_kind, workspace_id, namespace_id
) VALUES (
    '04000000-0000-0000-0000-000000000001', 'replay-tenant',
    '02000000-0000-0000-0000-000000000001', '03000000-0000-0000-0000-000000000001',
    'namespace', '01000000-0000-0000-0000-000000000001', 'replay-namespace'
);
SQL
run_forward hnb_empty 005 005
run_forward hnb_empty 021 021
run_forward hnb_empty 024 024
run_forward hnb_empty 060 060
# Storage 069-077 have explicit rollback/reapply coverage below. Replay the
# complete pre-storage chain here so legacy migrations execute against 060's
# current hierarchy without changing the independently maintained storage SQL.
run_forward hnb_empty 001 068
psql -X -v ON_ERROR_STOP=1 "$empty_url" >/dev/null <<'SQL'
DO $$
BEGIN
    IF to_regclass('projects') IS NOT NULL OR to_regclass('environments') IS NOT NULL THEN
        RAISE EXCEPTION 'legacy Project/Environment tables were recreated during replay';
    END IF;
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name IN ('namespaces', 'runtime_targets', 'scoped_role_bindings')
          AND column_name IN ('project_id', 'environment_id')
    ) THEN
        RAISE EXCEPTION 'legacy Project/Environment columns were recreated during replay';
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM namespaces
        WHERE id = 'replay-namespace'
          AND workspace_id = '01000000-0000-0000-0000-000000000001'
          AND tenant_id = 'replay-tenant'
    ) THEN
        RAISE EXCEPTION 'modern namespace data was not preserved during replay';
    END IF;
    IF NOT EXISTS (
        SELECT 1
        FROM scoped_role_bindings b
        JOIN tenant_memberships m
          ON m.tenant_id = b.tenant_id AND m.subject_id = b.subject_id
        WHERE b.id = '04000000-0000-0000-0000-000000000001'
          AND b.namespace_id = 'replay-namespace'
    ) THEN
        RAISE EXCEPTION 'scoped identity data was not preserved during replay';
    END IF;
END
$$;
SQL
psql -X -v ON_ERROR_STOP=1 "$(database_url hnb_empty)" --file "$script_dir/storage_desired_state_test.sql" >/dev/null
psql -X -v ON_ERROR_STOP=1 "$(database_url hnb_empty)" --file "$script_dir/storage_alert_rules_test.sql" >/dev/null
psql -X -v ON_ERROR_STOP=1 "$(database_url hnb_empty)" --file "$script_dir/storage_navigation_test.sql" >/dev/null
run_forward hnb_empty 078 078
psql -X -v ON_ERROR_STOP=1 "$(database_url hnb_empty)" --file "$script_dir/resource_cluster_navigation_test.sql" >/dev/null
for number in 077 076 075 074 073 072 071 070 069; do
    rollback=("$script_dir/../migrations/${number}_"*.rollback.sql)
    psql -X -v ON_ERROR_STOP=1 "$empty_url" --file "${rollback[0]}" >/dev/null
done
if [[ "$(psql -X -Aqt "$(database_url hnb_empty)" -c "SELECT path FROM console_routes WHERE route_name = 'container.instances.storage'")" != "/container/instances/storage" ]]; then
    echo "storage navigation rollback did not restore the legacy direct route" >&2
    exit 1
fi
if [[ "$(psql -X -Aqt "$(database_url hnb_empty)" -c "SELECT to_regclass('storage_backends') IS NULL")" != "t" ]]; then
    echo "storage desired-state rollback did not remove additive tables" >&2
    exit 1
fi
run_forward hnb_empty 069 077
psql -X -v ON_ERROR_STOP=1 "$empty_url" --file "$script_dir/storage_desired_state_test.sql" >/dev/null
psql -X -v ON_ERROR_STOP=1 "$empty_url" --file "$script_dir/storage_alert_rules_test.sql" >/dev/null
psql -X -v ON_ERROR_STOP=1 "$empty_url" --file "$script_dir/storage_navigation_test.sql" >/dev/null

echo "[2/6] 005/010 legacy shape mixed-version upgrade"
create_database hnb_legacy
run_forward hnb_legacy 001 020
legacy_url="$(database_url hnb_legacy)"
psql -X -v ON_ERROR_STOP=1 "$legacy_url" >/dev/null <<'SQL'
INSERT INTO tenants (id, name, display_name) VALUES
    ('tenant-a', 'tenant-a', 'Tenant A'),
    ('tenant-b', 'tenant-b', 'Tenant B');
CREATE TABLE users (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    email TEXT,
    display_name TEXT,
    password_hash TEXT NOT NULL,
    source TEXT NOT NULL,
    source_id TEXT,
    is_active BOOLEAN NOT NULL,
    labels JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO users (id, username, password_hash, source, is_active)
VALUES ('legacy-user-a', 'legacy-user-a', 'not-a-real-hash', 'local', true);
INSERT INTO roles (id, tenant_id, name)
VALUES ('legacy-role-a', 'tenant-a', 'readonly');
INSERT INTO user_roles (user_id, tenant_id, role_id, granted_by)
VALUES ('legacy-user-a', 'tenant-a', 'legacy-role-a', 'migration-test');
INSERT INTO projects (id, tenant_id, name) VALUES
    ('project-text-a', 'tenant-a', 'project-a');
INSERT INTO environments (id, tenant_id, project_id, name, env_type) VALUES
    ('environment-text-a', 'tenant-a', 'project-text-a', 'dev-a', 'development');
INSERT INTO runtime_targets (id, tenant_id, name, target_type) VALUES
    ('10000000-0000-0000-0000-000000000001', 'tenant-a', 'target-a', 'kubernetes');
INSERT INTO provider_registry (
    id, provider_id, provider_type, runtime_target_id, tenant_id, name
) VALUES (
    '20000000-0000-0000-0000-000000000001', 'provider-a', 'k8s_deploy',
    '10000000-0000-0000-0000-000000000001', 'tenant-a', 'Provider A'
);

-- Simulate the object left behind when the old 021 failed on projects.workspace_id.
CREATE TABLE workspaces (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    display_name TEXT,
    tenant_id TEXT NOT NULL,
    labels JSONB DEFAULT '{}',
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_workspaces_name ON workspaces(tenant_id, name);
SQL
run_forward hnb_legacy 021 021
psql -X -v ON_ERROR_STOP=1 "$legacy_url" >/dev/null <<'SQL'
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM projects p JOIN workspaces w ON w.id = p.workspace_id
        WHERE p.id = 'project-text-a' AND w.tenant_id = 'tenant-a'
    ) THEN
        RAISE EXCEPTION 'legacy project was not preserved/backfilled before hierarchy retirement';
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM environments
        WHERE id = 'environment-text-a' AND env_type = 'development' AND type = 'development'
    ) THEN
        RAISE EXCEPTION 'legacy environment mapping was not preserved before hierarchy retirement';
    END IF;
END
$$;

-- Simulate the object left behind when the old 022 failed on provider_registry.target_id.
CREATE TABLE extensions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    version TEXT NOT NULL,
    provider_type TEXT NOT NULL,
    workspace_id UUID REFERENCES workspaces(id) ON DELETE CASCADE,
    target_id TEXT,
    phase TEXT NOT NULL DEFAULT 'pending',
    manifest JSONB NOT NULL DEFAULT '{}',
    labels JSONB DEFAULT '{}',
    health_failures INTEGER NOT NULL DEFAULT 0,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
SQL
run_forward hnb_legacy 022 999
run_forward hnb_legacy 001 068

psql -X -v ON_ERROR_STOP=1 "$legacy_url" >/dev/null <<'SQL'
DO $$
BEGIN
    IF to_regclass('projects') IS NOT NULL OR to_regclass('environments') IS NOT NULL
       OR EXISTS (
            SELECT 1 FROM information_schema.columns
            WHERE table_schema = current_schema()
              AND table_name = 'runtime_targets'
              AND column_name IN ('project_id', 'environment_id')
       ) THEN
        RAISE EXCEPTION 'retired Project/Environment hierarchy survived migration 060';
    END IF;
    IF (SELECT data_type FROM information_schema.columns
        WHERE table_name = 'extensions' AND column_name = 'target_id') <> 'text'
       OR (SELECT data_type FROM information_schema.columns
        WHERE table_name = 'extensions' AND column_name = 'runtime_target_id') <> 'uuid' THEN
        RAISE EXCEPTION 'legacy extension target was cast or canonical target link is missing';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_workspaces_tenant') THEN
        RAISE EXCEPTION 'legacy workspaces table was not reconciled';
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM provider_registry
        WHERE provider_id = 'provider-a'
          AND tenant_id = 'tenant-a'
          AND runtime_target_id = target_id
          AND version = 1
    ) THEN
        RAISE EXCEPTION 'provider_registry canonical fields or alias were not preserved';
    END IF;
    IF NOT EXISTS (
        SELECT 1
        FROM identity_subjects s
        JOIN tenant_memberships m ON m.subject_id = s.id
        WHERE s.issuer = 'https://iam.hnb.local'
          AND s.external_subject = 'legacy-user-a'
          AND m.tenant_id = 'tenant-a'
          AND m.status = 'active'
    ) THEN
        RAISE EXCEPTION 'legacy user identity or tenant membership was not bridged';
    END IF;
END
$$;
SQL

echo "[3/6] tenant-safe constraints and immutable facts"
psql -X -v ON_ERROR_STOP=1 "$legacy_url" >/dev/null <<'SQL'
INSERT INTO identity_subjects (id, issuer, external_subject, subject_type)
VALUES ('30000000-0000-0000-0000-000000000001', 'https://issuer.example', 'alice', 'user');
INSERT INTO tenant_memberships (tenant_id, subject_id)
VALUES ('tenant-a', '30000000-0000-0000-0000-000000000001');
INSERT INTO scoped_roles (id, tenant_id, name)
VALUES ('40000000-0000-0000-0000-000000000001', 'tenant-a', 'operator');
INSERT INTO auth_refresh_tokens (token_hash, purpose, subject_id, membership_id, expires_at)
SELECT repeat('a', 64), 'refresh', s.id, m.id, now() + interval '1 hour'
FROM identity_subjects s
JOIN tenant_memberships m ON m.subject_id = s.id
WHERE s.issuer = 'https://iam.hnb.local' AND s.external_subject = 'legacy-user-a';

BEGIN;
SET CONSTRAINTS ALL DEFERRED;
INSERT INTO execution_plans (
    id, release_id, tenant_id, plan_digest, plan_json, runtime_intent_id
) VALUES (
    '50000000-0000-0000-0000-000000000001', 'release-legacy', 'tenant-a',
    'sha256:plan-a', '{}', '60000000-0000-0000-0000-000000000001'
);
INSERT INTO operations (
    id, tenant_id, plan_id, operation_type, initiated_by, idempotency_key, runtime_intent_id
) VALUES (
    '70000000-0000-0000-0000-000000000001', 'tenant-a',
    '50000000-0000-0000-0000-000000000001', 'deploy', 'alice', 'operation-a',
    '60000000-0000-0000-0000-000000000001'
);
INSERT INTO runtime_intents (
    id, tenant_id, subject_id, intent_kind, api_version, idempotency_key,
    semantic_digest, intent_document, runtime_target_id, execution_plan_id,
    operation_id, correlation_id
) VALUES (
    '60000000-0000-0000-0000-000000000001', 'tenant-a',
    '30000000-0000-0000-0000-000000000001', 'InstallRelease', 'hnb.io/v1',
    'intent-a', 'sha256:intent-a', '{}', '10000000-0000-0000-0000-000000000001',
    '50000000-0000-0000-0000-000000000001', '70000000-0000-0000-0000-000000000001',
    '80000000-0000-0000-0000-000000000001'
);
COMMIT;

INSERT INTO security_audit_events (
    id, tenant_id, subject_id, event_type, decision, action, runtime_intent_id,
    execution_plan_id, operation_id, correlation_id, outcome
) VALUES (
    '90000000-0000-0000-0000-000000000001', 'tenant-a',
    '30000000-0000-0000-0000-000000000001', 'intent_committed', 'allow', 'runtime.install',
    '60000000-0000-0000-0000-000000000001', '50000000-0000-0000-0000-000000000001',
    '70000000-0000-0000-0000-000000000001', '80000000-0000-0000-0000-000000000001', 'committed'
);
SQL

tenant_b_workspace="$(psql -X -Aqt "$legacy_url" -c "SELECT id FROM workspaces WHERE tenant_id = 'tenant-b' AND name = 'default'")"
expect_failure hnb_legacy "INSERT INTO runtime_targets (tenant_id, name, target_type, workspace_id) VALUES ('tenant-a', 'cross-tenant', 'kubernetes', '$tenant_b_workspace')"
expect_failure hnb_legacy "UPDATE runtime_intents SET semantic_digest = 'changed' WHERE id = '60000000-0000-0000-0000-000000000001'"
expect_failure hnb_legacy "DELETE FROM security_audit_events WHERE id = '90000000-0000-0000-0000-000000000001'"

psql -X -v ON_ERROR_STOP=1 "$legacy_url" >/dev/null <<'SQL'
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'signing_key_metadata'
          AND column_name ~ '(private|secret).*?(key|material)|(key|material).*?(private|secret)'
    ) THEN
        RAISE EXCEPTION 'private signing material column detected';
    END IF;
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'auth_refresh_tokens'
          AND column_name IN ('token', 'access_token', 'refresh_token')
    ) OR NOT EXISTS (
        SELECT 1 FROM auth_refresh_tokens
        WHERE token_hash = repeat('a', 64) AND purpose = 'refresh'
    ) THEN
        RAISE EXCEPTION 'refresh credential is not stored as a purpose-bound SHA-256 hash';
    END IF;
END
$$;
SQL

echo "[4/6] non-destructive rollback rehearsal"
for number in 027 026 025 024 023 022 021; do
    rollback=("$script_dir/../rollbacks/${number}_"*.rollback.sql)
    psql -X -v ON_ERROR_STOP=1 "$legacy_url" --file "${rollback[0]}" >/dev/null
done
psql -X -v ON_ERROR_STOP=1 "$legacy_url" >/dev/null <<'SQL'
DO $$
BEGIN
    IF (SELECT count(*) FROM operations WHERE id = '70000000-0000-0000-0000-000000000001') <> 1
       OR (SELECT count(*) FROM runtime_intents WHERE id = '60000000-0000-0000-0000-000000000001') <> 1
       OR (SELECT count(*) FROM security_audit_events WHERE id = '90000000-0000-0000-0000-000000000001') <> 1 THEN
        RAISE EXCEPTION 'rollback removed canonical operation evidence';
    END IF;
END
$$;
SQL

echo "[5/6] consistent snapshot restore"
create_database hnb_recovered
pg_dump --format=custom "$legacy_url" | pg_restore --exit-on-error --dbname "$(database_url hnb_recovered)"
psql -X -v ON_ERROR_STOP=1 "$(database_url hnb_recovered)" >/dev/null <<'SQL'
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM runtime_intents i
        JOIN operations o ON o.id = i.operation_id AND o.tenant_id = i.tenant_id
        JOIN security_audit_events a ON a.runtime_intent_id = i.id AND a.tenant_id = i.tenant_id
        WHERE i.id = '60000000-0000-0000-0000-000000000001'
          AND i.correlation_id = a.correlation_id
    ) THEN
        RAISE EXCEPTION 'recovered evidence chain is incomplete';
    END IF;
END
$$;
SQL

echo "[6/6] migration verification complete on PostgreSQL 16"
