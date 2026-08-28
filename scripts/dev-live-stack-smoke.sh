#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE_FILE="${ROOT_DIR}/deploy/docker-compose/compose.yml"
IDENTITY_DIR="${HNB_IDENTITY_KEYSET_DIR:-${TMPDIR:-/tmp}/hnb-dev-identity}"
ISSUER="${API_TOKEN_ISSUER:-https://issuer.dev.hnb.local}"
RESET_DB="${HNB_SMOKE_RESET_DB:-true}"
COMPOSE=(docker compose -f "${COMPOSE_FILE}")

require() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'missing required command: %s\n' "$1" >&2
    exit 1
  fi
}

generate_identity() {
  require openssl
  mkdir -p "${IDENTITY_DIR}"
  if [[ ! -f "${IDENTITY_DIR}/private.pem" || ! -f "${IDENTITY_DIR}/public.pem" ]]; then
    openssl ecparam -name prime256v1 -genkey -noout -out "${IDENTITY_DIR}/private.pem"
    openssl pkcs8 -topk8 -nocrypt -in "${IDENTITY_DIR}/private.pem" -out "${IDENTITY_DIR}/private.pkcs8.pem"
    mv "${IDENTITY_DIR}/private.pkcs8.pem" "${IDENTITY_DIR}/private.pem"
    openssl ec -in "${IDENTITY_DIR}/private.pem" -pubout -out "${IDENTITY_DIR}/public.pem"
    chmod 644 "${IDENTITY_DIR}/public.pem"
  fi
  # Dev-only disposable key: apiserver runs as UID 65532 and enforces private key mode 0600.
  if chown 65532:65532 "${IDENTITY_DIR}/private.pem" 2>/dev/null; then
    chmod 600 "${IDENTITY_DIR}/private.pem"
  else
    printf 'failed to chown %s to 65532:65532; run this script with a user allowed to chown dev identity files\n' "${IDENTITY_DIR}/private.pem" >&2
    exit 1
  fi

  local not_before not_after
  not_before="$(date -u -d '1 hour ago' +%Y-%m-%dT%H:%M:%SZ)"
  not_after="$(date -u -d '30 days' +%Y-%m-%dT%H:%M:%SZ)"
  cat >"${IDENTITY_DIR}/manifest.json" <<JSON
{
  "issuer": "${ISSUER}",
  "generation": 1,
  "activeKeyId": "dev-1",
  "keys": {
    "dev-1": {
      "publicKeyPath": "/var/run/hnb-identity/public.pem",
      "status": "active",
      "notBefore": "${not_before}",
      "notAfter": "${not_after}"
    }
  }
}
JSON
  chmod 644 "${IDENTITY_DIR}/manifest.json"
}

export_env() {
  export HNB_IDENTITY_KEYSET_DIR="${IDENTITY_DIR}"
  export API_TOKEN_ISSUER="${ISSUER}"
  export API_TOKEN_AUDIENCES="hnb-apiserver,hnb-platform-api"
  export HTTP_PROXY=""
  export HTTPS_PROXY=""
  export ALL_PROXY=""
  export http_proxy=""
  export https_proxy=""
  export all_proxy=""
  export NO_PROXY="*"
  export no_proxy="*"
  export HNB_BOOTSTRAP_ADMIN_PASSWORD="${HNB_BOOTSTRAP_ADMIN_PASSWORD:-hnb123}"
  export HNB_KUBERNETES_PROVIDER_TOKEN_FILE="${IDENTITY_DIR}/dev-kubernetes-provider.jwt"
  export HNB_EDGE_PROVIDER_TOKEN_FILE="${IDENTITY_DIR}/dev-edge-provider.jwt"
  export APP_MARKET_DB_DSN="postgres://hnb:hnb123@postgres:5432/hnb?sslmode=disable"
  : >"${HNB_KUBERNETES_PROVIDER_TOKEN_FILE}"
  : >"${HNB_EDGE_PROVIDER_TOKEN_FILE}"
  chmod 600 "${HNB_KUBERNETES_PROVIDER_TOKEN_FILE}" "${HNB_EDGE_PROVIDER_TOKEN_FILE}"
}

apply_migrations() {
  for migration in "${ROOT_DIR}"/database/postgresql/migrations/[0-9][0-9][0-9]_*.sql; do
    case "${migration}" in
      *.rollback.sql) continue ;;
    esac
    printf 'applying migration %s\n' "$(basename "${migration}")"
    "${COMPOSE[@]}" exec -T postgres psql -v ON_ERROR_STOP=1 -U hnb -d hnb <"${migration}"
  done
}

reset_database() {
  if [[ "${RESET_DB}" != "true" ]]; then
    return 0
  fi
  printf 'resetting local smoke database schema; set HNB_SMOKE_RESET_DB=false to skip\n'
  "${COMPOSE[@]}" exec -T postgres psql -v ON_ERROR_STOP=1 -U hnb -d hnb <<'SQL'
DROP SCHEMA IF EXISTS public CASCADE;
CREATE SCHEMA public;
GRANT ALL ON SCHEMA public TO hnb;
GRANT ALL ON SCHEMA public TO public;
SQL
}

wait_http() {
  local url="$1"
  local name="$2"
  local i
  for i in $(seq 1 60); do
    if curl --noproxy '*' -fsS "${url}" >/dev/null 2>&1; then
      printf '%s is ready: %s\n' "${name}" "${url}"
      return 0
    fi
    sleep 2
  done
  printf '%s did not become ready: %s\n' "${name}" "${url}" >&2
  return 1
}

wait_postgres() {
  local i
  for i in $(seq 1 60); do
    if "${COMPOSE[@]}" exec -T postgres pg_isready -U hnb >/dev/null 2>&1; then
      printf 'postgres is ready\n'
      return 0
    fi
    sleep 2
  done
  printf 'postgres did not become ready\n' >&2
  return 1
}

main() {
  require docker
  require curl
  generate_identity
  export_env

  "${COMPOSE[@]}" --profile lifecycle up -d postgres nats
  wait_postgres
  reset_database
  apply_migrations
  "${COMPOSE[@]}" --profile lifecycle up -d platform-api apiserver extension-controller
  wait_http http://localhost:8080/health apiserver
  wait_http http://localhost:8084/metrics extension-controller
  printf '\nLive-stack smoke passed.\n'
  printf 'Identity fixture: %s\n' "${IDENTITY_DIR}"
}

main "$@"
