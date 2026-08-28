# Portal Access: API SSE/WebSocket, Not Direct NATS

## Constraint

Portal MUST NOT connect directly to NATS. All progress and event data flows through the Platform API, which enforces tenant isolation and authorization.

## Architecture

```
Portal (Browser)
     |
     | HTTPS / WSS
     v
Platform API (Gateway)
     |
     |--- Tenant auth + RBAC
     |--- Read Model queries
     |--- SSE/WebSocket streams
     |
     v
[PostgreSQL Read Model]  [NATS JetStream] (internal only)
```

## API Endpoints

### SSE Stream for Real-Time Progress

```
GET /api/v1/operations/{id}/events?stream=sse
Authorization: Bearer <token>
Accept: text/event-stream
```

Response:
```
event: progress
data: {"operationId":"...","totalSteps":5,"completedSteps":2,"status":"in_progress"}

event: state-changed
data: {"operationId":"...","previousState":"in_progress","newState":"succeeded"}
```

### WebSocket for Bidirectional Updates

```
WSS /api/v1/operations/{id}/events?stream=websocket
Authorization: Bearer <token>
```

### Polling Fallback

```
GET /api/v1/operations/{id}
Authorization: Bearer <token>
Response: { "id": "...", "status": "in_progress", "steps": [...], "progress": {...} }
```

## Authorization

Every API call:
1. Verifies JWT/session token
2. Extracts tenant_id from token
3. Verifies operation belongs to tenant
4. Returns only authorized data

## Network Policy

- NATS cluster is in a private subnet
- No ingress from Portal/CLI network
- Only Platform API service account has NATS access
- Kubernetes NetworkPolicy denies all non-API traffic to NATS

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: nats-private
  namespace: hnb-messaging
spec:
  podSelector:
    matchLabels:
      app: nats-jetstream
  policyTypes:
    - Ingress
  ingress:
    - from:
        - podSelector:
            matchLabels:
              app: platform-api
      ports:
        - port: 4222
```

## Tenant Isolation Verification

| Scenario | Expected Result |
|----------|----------------|
| Tenant A queries operation progress | Only Tenant A's operations visible |
| Tenant B attempts to access Tenant A's SSE stream | 403 Forbidden |
| Unauthenticated request to events endpoint | 401 Unauthorized |
| Direct NATS connection from Portal browser | Network connection refused |
| Direct NATS connection from Portal backend | Service account not granted NATS access |
| CLI tool connects to NATS | Configuration does not expose NATS endpoint |