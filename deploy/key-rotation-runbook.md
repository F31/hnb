# Signing Key Rotation Runbook

## Safety Contract

- The manifest Secret contains `manifest.json` and public P-256 PEM files only.
  Never put a private key, private-key path, credential, or key bytes in the
  manifest or an ordinary database table.
- Apiserver mounts the active private key from a separate Secret at
  `/var/run/hnb-signing/active-private.pem`. Updating that Secret and the
  manifest Secret uses Kubernetes' atomic projected-volume symlink exchange.
- `API_TOKEN_KEY_RELOAD_INTERVAL` must be 1 through 60 seconds. Five seconds is
  the deployment default and the expected emergency propagation bound.
- Increase `generation` for every manifest lifecycle change. Never reuse or
  decrease a generation, including during rollback.
- A key's `publicKeyPath`, `notBefore`, and `notAfter` are immutable after the
  key first appears. Generate a new `kid` instead of changing those fields.

Example shape (paths and times are illustrative; no keys are committed):

```json
{
  "issuer": "https://identity.example",
  "generation": 42,
  "activeKeyId": "k2",
  "keys": {
    "k1": {
      "publicKeyPath": "/var/run/hnb-identity/k1-public.pem",
      "status": "retiring",
      "notBefore": "2026-07-27T00:00:00Z",
      "notAfter": "2026-07-28T00:00:00Z"
    },
    "k2": {
      "publicKeyPath": "/var/run/hnb-identity/k2-public.pem",
      "status": "active",
      "notBefore": "2026-07-27T12:00:00Z",
      "notAfter": "2026-07-29T12:00:00Z"
    }
  }
}
```

## Routine Rotation

1. Generate K2 outside the repository. Verify it is P-256, protect its private
   key, and choose a unique `kid` and bounded validity window.
2. Publish generation N with K1 `active` and K2 `pending`. Include both public
   PEM files. Do not change the apiserver active-private Secret yet.
3. Confirm every apiserver, platform-api, app-market, kubernetes-provider,
   edge-provider, and tunnel-server replica logs `key manifest generation N
   loaded`. A pending K2 is visible but cannot verify or sign.
4. Atomically update the apiserver active-private Secret to K2 and publish
   generation N+1 with K1 `retiring`, K2 `active`, and `activeKeyId` K2. A
   transient public/private mismatch is safe: reload fails and the previous
   good snapshot remains active until both projected volumes converge.
5. Confirm every replica logs generation N+1. Confirm new tokens have `kid=K2`,
   K2 verifies everywhere, and existing K1 tokens still verify during overlap.
6. Wait at least `maximum access-token TTL + accepted clock skew` after the last
   K1 token could have been issued. The current maximum TTL is 60 seconds and
   verifier clock-skew allowance is zero, so the protocol minimum is 60
   seconds; add the operator's fleet clock-safety margin.
7. Publish generation N+2 with K1 `expired` and K2 `active`. K1's `notAfter`
   must already have elapsed. Confirm all replicas loaded N+2 before optionally
   removing K1 in a later, higher generation.

## Emergency Revocation

1. Stop distributing the compromised K1 private key and create K2.
2. Atomically update the apiserver active-private Secret to K2 and publish the
   next generation with K1 `revoked`, K2 `active`, and `activeKeyId` K2. Do not
   use a retiring overlap for compromised K1.
3. Confirm all replicas load the generation within the configured polling
   bound. A successfully loaded snapshot rejects K1 immediately; it does not
   wait for token expiry.
4. Verify K1 tokens fail at all six verifier entry points and K2 tokens succeed.
   Preserve the manifest generation, service logs, and apiserver lifecycle rows
   as incident evidence.

## Failure And Rollback

- Startup fails when the manifest is absent or invalid. There is no production
  static-key environment fallback.
- Unknown fields, duplicate JSON fields, unsafe paths/permissions/sizes,
  non-P-256 keys, invalid windows/statuses, multiple active keys, private/public
  mismatch, forbidden status transitions, generation reuse, and generation
  rollback all fail closed.
- A failed runtime reload returns an error, increments `KeyReloadStats.Failures`,
  logs the failure, and retains the previous immutable snapshot. It never
  partially installs keys.
- Roll back a bad manifest by publishing corrected content at a new, higher
  generation. Never restore an older Secret revision or lower generation.
- Binary rollback must retain a manifest-aware version and must not re-enable
  `API_TOKEN_VERIFICATION_KEYS`. If that cannot be guaranteed, keep traffic
  blocked instead of restoring static keys.
- Only apiserver records successful generations and status transitions in the
  IAM `signing_key_metadata` and append-only
  `signing_key_lifecycle_events` tables. Verifier-only planes do not write the
  IAM database.

## Drill Status

This runbook documents the procedures only. No real routine or emergency key
rotation drill was performed for task 14; OpenSpec task 25 remains open.
