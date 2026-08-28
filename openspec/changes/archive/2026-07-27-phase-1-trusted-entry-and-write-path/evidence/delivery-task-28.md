# Delivery: Task 28 - SLOs and Alert Definitions

## Scope
P1-ING-006 — Service level objectives and alerting rules for auth, policy, intent, operation, and outbox.

## SLO Definitions

### Auth Failure Rate (P1-ING-001)
- **Objective**: 99.9% of auth requests succeed within MaxAccessTokenTTL (60s)
- **Measurement**: `/apiserver/auth/verify/failures_total` per issuer
- **Alert threshold**: > 0.1% failure rate over 5-minute window
```yaml
alerts:
  - name: HighAuthFailureRate
    expr: rate(auth_verify_failures_total[5m]) / rate(auth_verify_attempts_total[5m]) > 0.001
    for: 5m
    labels:
      severity: critical
```

### Authorization Decision Latency (P1-ING-003)
- **Objective**: p95 < 50ms for Evaluate/EvaluateRoute decisions
- **Measurement**: Time from TrustedContext injection to HTTP handler access
- **Alert threshold**: p95 > 50ms sustained over 10 minutes
```yaml
alerts:
  - name: HighAuthorizationLatency
    expr: histogram_quantile(0.95, rate(auth_decision_duration_seconds_bucket[5m])) > 0.050
    for: 10m
    labels:
      severity: warning
```

### Intent Validation & Planning Throughput (P1-WRITE-001, P1-WRITE-002)
- **Objective**: 99.5% of intent submissions complete validation+planning within 5 seconds
- **Measurement**: Duration from POST /v1/intents body received to ExecutionPlan returned
- **Alert threshold**: > 0.5% failures over 10 minutes
```yaml
alerts:
  - name: IntentPlanningTimeoutRate
    expr: rate(intent_planning_timeout_total[10m]) / rate(intent_submissions_total[10m]) > 0.005
    for: 10m
    labels:
      severity: warning
```

### Operation Commit Success Rate (P1-WRITE-003, P1-WRITE-004)
- **Objective**: 99.9% of SubmitIntent/SubmitOperation commits succeed
- **Measurement**: operations table insert + outbox event emission atomic success
- **Alert threshold**: Transaction rollback rate > 0.1%
```yaml
alerts:
  - name: OperationCommitFailureRate
    expr: rate(operation_commit_failures_total[5m]) / rate(operation_submissions_total[5m]) > 0.001
    for: 5m
    labels:
      severity: critical
```

### Outbox Processing Lag (P1-WRITE-005, P1-ING-006)
- **Objective**: Outbox events consumed within 30 seconds of emission
- **Measurement**: time_difference between outbox_events.occurred_at and consumer processing
- **Alert threshold**: Median lag > 15 seconds
```yaml
alerts:
  - name: HighOutboxLag
    expr: percentile_hist(outbox_event_lag_seconds[5m], 50) > 15
    for: 10m
    labels:
      severity: warning
```

### Token Expiry Bound (P1-ING-005)
- **Objective**: All access tokens revoked within 60-second bound
- **Measurement**: Time from key rotation to new token requirement enforced
- **Alert threshold**: Any token still valid beyond notAfter window
```yaml
alerts:
  - name: TokenRevocationBoundExceeded
    expr: increase(token_revocation_boundary_violations_total[1h]) > 0
    for: 1h
    labels:
      severity: critical
```
