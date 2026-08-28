#!/usr/bin/env bash
set -euo pipefail

workspace="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
matches="$(rg --line-number --glob '*.go' '\b(nats|gnats)\.Connect\(' "$workspace" || true)"
unexpected="$(printf '%s\n' "$matches" | rg -v '/pkg/(messaging/nats\.go|integration/env\.go):' || true)"

if [[ -n "$unexpected" ]]; then
  printf 'production code must use pkg/messaging.ConnectNATSFromEnv:\n%s\n' "$unexpected" >&2
  exit 1
fi

allowed_count="$(printf '%s\n' "$matches" | rg -c '/pkg/(messaging/nats\.go|integration/env\.go):' || true)"
if [[ "$allowed_count" != "2" ]]; then
  printf 'expected only pkg/messaging and pkg/integration direct NATS connections, found:\n%s\n' "$matches" >&2
  exit 1
fi
