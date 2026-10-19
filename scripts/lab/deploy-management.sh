#!/usr/bin/env bash
# Create lab-only management credentials and deploy the chart on the management host.
# Secrets live only in the cluster; this script never writes them to the repo.
set -euo pipefail

usage() { echo "usage: deploy-management.sh <rendered.yaml> [infra|full]" >&2; exit 2; }
rendered=${1:?$(usage)}
phase=${2:-full}
[[ $phase == infra || $phase == full ]] || usage
[[ -r $rendered ]] || usage
namespace=hnb-system
private=/var/lib/hnb-lab/management-secrets
umask 077

kubectl create namespace "$namespace" --dry-run=client -o yaml | kubectl apply -f - >/dev/null
install -d -m 0700 "$private"

secret() {
  local name=$1; shift
  kubectl -n "$namespace" get secret "$name" >/dev/null 2>&1 && return 0
  kubectl -n "$namespace" create secret generic "$name" "$@" --dry-run=client -o yaml | kubectl apply -f - >/dev/null
}

password() { openssl rand -hex 24; }
hexkey() { openssl rand -hex 32; }

if [[ ! -f $private/ca.crt ]]; then
  openssl req -x509 -newkey rsa:2048 -nodes -days 30 -subj /CN=hnb-lab-nats-ca \
    -keyout "$private/ca.key" -out "$private/ca.crt" >/dev/null 2>&1
  openssl req -newkey rsa:2048 -nodes -subj /CN=nats \
    -keyout "$private/nats.key" -out "$private/nats.csr" >/dev/null 2>&1
  printf 'subjectAltName=DNS:nats,DNS:nats-headless,DNS:*.hnb-system.svc,DNS:*.hnb-system.svc.cluster.local,IP:127.0.0.1\nextendedKeyUsage=serverAuth,clientAuth\n' > "$private/nats.ext"
  openssl x509 -req -in "$private/nats.csr" -CA "$private/ca.crt" -CAkey "$private/ca.key" \
    -CAcreateserial -days 30 -extfile "$private/nats.ext" -out "$private/nats.crt" >/dev/null 2>&1
  chmod 0600 "$private"/*
fi
kubectl -n "$namespace" get secret hnb-nats-tls >/dev/null 2>&1 || \
  kubectl -n "$namespace" create secret generic hnb-nats-tls \
    --from-file=tls.crt="$private/nats.crt" --from-file=tls.key="$private/nats.key" \
    --from-file=ca.crt="$private/ca.crt" --dry-run=client -o yaml | kubectl apply -f - >/dev/null

if [[ ! -f $private/manifest.json ]]; then
  openssl ecparam -name prime256v1 -genkey -noout -out "$private/private-sec1.pem"
  openssl pkcs8 -topk8 -nocrypt -in "$private/private-sec1.pem" -out "$private/private.pem"
  openssl ec -in "$private/private.pem" -pubout -out "$private/public.pem" >/dev/null 2>&1
  not_before=$(date -u -d '5 minutes ago' +%Y-%m-%dT%H:%M:%SZ)
  not_after=$(date -u -d '30 days' +%Y-%m-%dT%H:%M:%SZ)
  printf '{"issuer":"https://issuer.hnb.test/","generation":1,"activeKeyId":"lab-1","keys":{"lab-1":{"publicKeyPath":"public.pem","status":"active","notBefore":"%s","notAfter":"%s"}}}\n' \
    "$not_before" "$not_after" > "$private/manifest.json"
  chmod 0600 "$private/manifest.json" "$private/private.pem" "$private/public.pem" "$private/private-sec1.pem"
fi
kubectl -n "$namespace" get secret hnb-identity-keys >/dev/null 2>&1 || \
  kubectl -n "$namespace" create secret generic hnb-identity-keys \
    --from-file=manifest.json="$private/manifest.json" --from-file=public.pem="$private/public.pem" \
    --dry-run=client -o yaml | kubectl apply -f - >/dev/null
kubectl -n "$namespace" get secret hnb-identity-private >/dev/null 2>&1 || \
  kubectl -n "$namespace" create secret generic hnb-identity-private \
    --from-file=tls.key="$private/private.pem" --dry-run=client -o yaml | kubectl apply -f - >/dev/null

if ! kubectl -n "$namespace" get secret hnb-pg-admin >/dev/null 2>&1; then
  admin=$(password); platform=$(password); market=$(password); artifact=$(password)
  secret hnb-pg-admin --from-literal=password="$admin"
  secret hnb-pg-platform --from-literal=password="$platform" \
    --from-literal=dsn="postgres://hnb_platform:${platform}@postgres:5432/hnb_platform?sslmode=disable"
  secret hnb-pg-market --from-literal=password="$market" \
    --from-literal=dsn="postgres://hnb_market:${market}@postgres:5432/hnb_market?sslmode=disable"
  secret hnb-pg-artifact --from-literal=password="$artifact" \
    --from-literal=dsn="postgres://hnb_artifact:${artifact}@postgres:5432/hnb_artifact?sslmode=disable"
fi

for plane in platform market artifact; do
  if ! kubectl -n "$namespace" get "secret/hnb-${plane}-nats" >/dev/null 2>&1; then
    secret "hnb-${plane}-nats" --from-literal=user="$plane" --from-literal=password="$(password)"
  fi
done
secret hnb-master-key --from-literal=key="$(hexkey)"
{ kubectl -n "$namespace" get secret hnb-admin >/dev/null 2>&1 || secret hnb-admin --from-literal=password="$(password)"; }
{ kubectl -n "$namespace" get secret hnb-stale-challenge >/dev/null 2>&1 || secret hnb-stale-challenge --from-literal=challenge.key="$(openssl rand -base64 48)"; }
{ kubectl -n "$namespace" get secret hnb-placeholder-tokens >/dev/null 2>&1 || secret hnb-placeholder-tokens --from-literal=placeholder=unused; }

kubectl -n "$namespace" apply --server-side --field-manager=hnb-lab -f "$rendered"
if [[ $phase == infra ]]; then
  kubectl -n "$namespace" rollout status statefulset/postgresql --timeout=300s
  kubectl -n "$namespace" rollout status statefulset/nats --timeout=300s
  echo "infra ready"
  exit 0
fi

for workload in deployment/apiserver deployment/platform-api deployment/operation-worker \
  deployment/extension-controller deployment/app-market deployment/cluster-router \
  deployment/tunnel-server deployment/portal statefulset/postgresql statefulset/nats; do
  kubectl -n "$namespace" rollout status "$workload" --timeout=300s
done
kubectl -n "$namespace" get pods -o wide
