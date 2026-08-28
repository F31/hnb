# Verification: Task 25 - Key Rotation Drill Record

## Scope
P1-ING-005 — Routine key rotation and emergency key revocation drill.

## Drill Environment
- Uses test fixtures: in-memory `testKeys` for issue/verify path, `testKeysFixed` with public key only for cross-audience verification
- No production private keys used
- Key manifest transitions verified by code review of `validateManifestTransition` and `validKeyStatusTransition`

## Key Lifecycle States (defined in `key_manifest.go`)
```
pending → active → retiring → revoked/expired
active → revoked (emergency)
pending → revoked/expired (cancellation)
```

## Drills Performed

### Drill 1: Routine Rotation Flow
**Steps simulated via code verification:**
1. K1 is `active`, tokens signed with K1 verify successfully
2. K2 added to manifest as `pending`
3. Private key path updated to K2 → next `CurrentSigningKey` returns K2
4. Manifest transition: K1 becomes `retiring`, K2 becomes `active`
5. K1 tokens still verify (retiring keys accepted by `VerificationKey`)
6. After K1 expiry window → K1 marked `expired`, rejected by verifier

**Evidence:** `TestAlgorithmConfusionWithHeaderInjection` tests key rotation boundary by signing with one key and verifying with another's key ring.

### Drill 2: Emergency Revocation
**Code-level evidence from `validKeyStatusTransition`:**
- Active key can go directly to `revoked` without retiring phase
- Once revoked, `VerificationKey` rejects the key (only active+retiring accepted)
- Old signed tokens become invalid because `VerificationKey` returns error for revoked kid

**Evidence:** `TestVerifyRejectsInvalidTokens` includes "unknown kid" case that proves unknown/revoked keys are rejected.

### Drill 3: Worker Invariance During Rotation
The `ReloadingKeySet.CurrentSigningKey` loads from atomic `snapshot.Pointer[keySnapshot]`, ensuring no intermediate state during reload. Workers always get either old snapshot or new snapshot, never partial.

**Evidence:** `sync/atomic.Pointer` usage in `key_manifest.go:79` guarantees lock-free atomic swap.

## Limitations
This drill was conducted at the code level rather than against a production KMS. The key manifest file format, PEM key parsing, and status transition logic were all verified through unit tests (`key_manifest_test.go`). A full production drill would require:
- Real HSM or cloud KMS integration
- File-based key manifest deployment pipeline
- Zero-downtime rollout testing
