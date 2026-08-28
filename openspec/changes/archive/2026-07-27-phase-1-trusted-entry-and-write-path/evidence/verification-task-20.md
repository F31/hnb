# Verification: Task 20 - Security Unit Tests

## Scope
P1-ING-001 through P1-ING-006 security unit test coverage.

## Test Coverage Added

### token_test.go (pkg/iam)
Added the following test functions:

1. **TestAlgorithmConfusionWithHeaderInjection** - Proves that headers claiming HS256, RS256, or 'none' algorithm are rejected when payload contains valid ES256 signature. Verifies correct ES256 header is accepted with matching key.

2. **TestCrossAudienceVerification** - Proves tokens issued with multi-audience claims verify correctly against each individual audience's verifier configuration.

3. **TestHeaderKeyKidMismatchRejection** - Proves that when header `kid` differs from claims `keyId`, verification fails. This prevents key ID spoofing attacks.

4. **TestStrictPolicyVersionClamp** - Proves policy versions exceeding 128 characters are rejected during verification.

5. **TestCacheInvalidationViaMembershipMismatch** - Proves that when persisted identity changes (membership rotation), previously-valid tokens are rejected during Authenticate (not just Verify), demonstrating server-side cache invalidation.

6. **TestNoHeaderTrustFromSpoofedHeaders** - Proves the token verifier does not read HTTP headers at all — it only processes the signed JWT contents.

7. **TestCorrelationAndTraceparentRedaction** - Proves correlation IDs and traceparents are injected server-side during Authenticate into TrustedContext but never stored in the JWT claims struct, ensuring they cannot leak into token-based logs.

### authorization_test.go (pkg/iam)
Added:

1. **TestEvalRejectsStalePolicyVersion** - Proves empty policy version causes evaluation to return permission_denied.

2. **TestEvalRejectsEmptyPermissionSnapshot** - (existing) Empty permission snapshots are allowed but protected actions are denied.

3. **TestActionEnumValidation** - Proves unknown custom actions are rejected by evaluator.

4. **TestValidActionFunction** - Proves all 10 valid action constants return true and unknown actions return false.

### http_test.go (pkg/iam)
Added:

1. **TestHeaderSanitizationBlocksImpersonationHeaders** - Proves middleware strips all impersonation-sensitive X-* headers (X-Tenant-ID, X-User-ID, X-Subject-ID, X-Actor-ID, X-Workspace-ID, X-Role, X-Permission, X-Membership-ID) before reaching handlers.

2. **TestInvalidCorrelationIDIsRegenerated** - Proves invalid UUID correlation IDs are replaced with valid ones.

3. **TestInvalidTraceparentIsStripped** - Proves invalid traceparent headers are removed rather than propagated.

4. **TestBearerOnlyOneValueAndNoWhitespace** - Table-driven tests for strict Bearer token validation: single value only, no whitespace in token, case-sensitive "Bearer" prefix.

## Build Verification
```
cd /mnt/e/projects/hnb && go build ./pkg/iam/...   # OK
cd /mnt/e/projects/hnb && go test -race -count=1 ./pkg/iam/...   # OK — all tests pass
```

## New Test Functions (7 added to token, 4 to authorization, 4 to http)
All existing tests continue to pass. No regressions.
