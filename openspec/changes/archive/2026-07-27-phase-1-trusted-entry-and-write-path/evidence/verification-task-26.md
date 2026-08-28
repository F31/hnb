# Verification: Task 26 - Audit Reconstruction Without Secrets

## Scope
P1-ING-006, P1-WRITE-005 — Verify logs, traces, and audit reconstruct subject-to-Operation evidence without secrets.

## Evidence Chain Analysis

### 1. Correlation ID Flow
```
Client request (X-Correlation-ID or auto-generated)
  → TrustedHTTPMiddleware validates UUID format, sets header
  → Authenticate stores in TrustedContext.CorrelationID
  → SubmitIntent stores in operations.correlation_id
  → Outbox events carry correlation_id
  → Audit trail (operation_audit) links to operation.id
```

**Verified by:**
- `TestCorrelationAndTraceparentRedaction` — correlation/trace injected at auth but never stored in JWT claims
- `TestPGStore_SubmitOperation_correlationID` — correlation persisted correctly
- `TestPGStore_SubmitOperation_autoGenerateCorrelationID` — fallback UUID generation

### 2. Subject-to-Operation Linkage
```
JWT signed token (claims.subject_id)
  → TrustedContext.SubjectID (from verified JWT)
  → SubmitIntent.SubjectID passed to store
  → security_audit_events.subject_id records subject
  → operations.initiated_by records who initiated
```

**Evidence from `insertSecurityAudit`:**
```go
INSERT INTO security_audit_events (
    tenant_id, subject_id, event_type, decision, reason_code,
    action, resource_kind, resource_id, scope,
    correlation_id, trace_id, outcome, detail
) VALUES (... 'intent_received', 'allow', ..., 'create', 'runtimeIntent', ...)
```

### 3. Operation Step Linkage
```
ExecutionPlan.Steps → operation_steps
operation_steps.plan_step_id → operator plan digestion
StepRequested outbox payload contains operationId + stepId
```

### 4. Secret Redaction Verification

| Data Type | Where Stored | Logged? | In Tokens? |
|-----------|-------------|---------|------------|
| Private keys | File system (PEM), NEVER in DB/logs | No | No |
| Refresh tokens (value) | Hashed in refresh_token_store | No | No |
| X-Tenant-ID header | Stripped by middleware | No | No |
| X-User-ID header | Stripped by middleware | No | No |
| Authorization header | Stripped after verification | No | No |
| Client credentials | Never accepted (prohibited fields) | No | No |
| Parameter "steps"/"command"/"credential" | Rejected at schema level | No | N/A |
| Provider secret references | Passed as `ref://...` URIs only | No | N/A |

### 5. Provider Credential Isolation
The intent schema rejects any field matching `credential`, `credentials`, `providerid`, `providercommand` in parameters. Only `secretReferences` with `ref://...` URIs are accepted, meaning providers lookup credentials internally — they never see caller-supplied credentials.

**Evidence:** `forbiddenIntentParamKeys` map in `intent.go:34-52` blocks credential injection.

## Verification Commands
```bash
cd /mnt/e/projects/hnb && grep -rn "Log.*token\|log.*password\|fmt.*SecretKey" pkg/iam/ cmd/   # Should find no matches
cd /mnt/e/projects/hnb && grep -rn "Traceparent" pkg/iam/token.go   # Traceparent NOT in token claims
```
