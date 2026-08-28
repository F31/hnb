# Tasks: ai-extension

## 1. AI Extension Service Skeleton (AI-001)

- [x] 1.1 Create cmd/ai-extension/ with health check endpoint
- [x] 1.2 Add go.mod and go.work entry

## 2. Write Operation Bypass Detection (AI-004)

- [x] 2.1 Add AI source header check middleware in platform-api
- [x] 2.2 Reject AI-initiated write requests without operationId reference

## 3. High-Risk Automation Limits (AI-005)

- [ ] 3.1 Add risk level check for AI-initiated operations
- [ ] 3.2 Implement cooldown and circuit breaker for auto-remediation