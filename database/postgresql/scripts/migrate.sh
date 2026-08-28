#!/usr/bin/env bash
set -euo pipefail

database_url="${1:-${DATABASE_URL:-}}"
if [[ -z "$database_url" ]]; then
    echo "usage: DATABASE_URL=postgresql://... bash database/postgresql/scripts/migrate.sh" >&2
    exit 64
fi

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
migrations_dir="$(cd "$script_dir/../migrations" && pwd)"
from="${MIGRATION_FROM:-001}"
to="${MIGRATION_TO:-999}"

shopt -s nullglob
migrations=("$migrations_dir"/[0-9][0-9][0-9]_*.sql)
if [[ ${#migrations[@]} -eq 0 ]]; then
    echo "no forward migrations found in $migrations_dir" >&2
    exit 1
fi

for migration in "${migrations[@]}"; do
    base="$(basename "$migration")"
    case "$base" in
        *.rollback.sql) continue ;;
    esac

    number="${base%%_*}"
    if ((10#$number < 10#$from || 10#$number > 10#$to)); then
        continue
    fi

    echo "=== $base ==="
    psql -X -v ON_ERROR_STOP=1 "$database_url" --file "$migration"
done
