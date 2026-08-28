#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
database_url="${DATABASE_URL:-}"
tenant_id=""
target_id=""
dry_run=true

usage() {
    printf '%s\n' "usage: DATABASE_URL=postgresql://... $0 --tenant TENANT --target UUID [--apply]" >&2
}

while (($#)); do
    case "$1" in
        --tenant) tenant_id="${2:-}"; shift 2 ;;
        --target) target_id="${2:-}"; shift 2 ;;
        --apply) dry_run=false; shift ;;
        --dry-run) dry_run=true; shift ;;
        *) usage; exit 64 ;;
    esac
done

if [[ -z "$database_url" || -z "$tenant_id" || -z "$target_id" ]]; then
    usage
    exit 64
fi

# This tool has no Kubernetes credentials or client path. Its sole side effect,
# with --apply, is one PostgreSQL transaction over desired-state tables.
exec psql -X --set=ON_ERROR_STOP=1 \
    --set=tenant_id="$tenant_id" \
    --set=target_id="$target_id" \
    --set=dry_run="$dry_run" \
    "$database_url" --file "$script_dir/import-observed-storage-classes.sql"
