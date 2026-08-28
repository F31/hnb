# Message Size, Forbidden Fields, and Payload Reference Gate

## Message Size Limits

| Message Type | Max Envelope Size | Max Payload Size | Max Total Size |
|-------------|-------------------|------------------|----------------|
| Command (step-requested) | 4 KB | 60 KB | 64 KB |
| Event (step-completed) | 4 KB | 60 KB | 64 KB |
| Event (state-changed) | 1 KB | 3 KB | 4 KB |
| Notification (progress) | 1 KB | 3 KB | 4 KB |

## Enforcement Points

1. **Producer-side (Outbox Relay)**: Before publishing to JetStream, validate message size against per-type limits. Reject with error if exceeded.

2. **Broker-side (JetStream)**: Configure `max_msg_size` per stream:
   - `commands` stream: max_msg_size = 2 MB
   - `domain-events` stream: max_msg_size = 2 MB
   - `notifications` stream: max_msg_size = 1 MB

3. **Consumer-side**: Verify message size on receipt; log warning if near limit.

## Forbidden Fields Gate

The following fields MUST NOT appear in any message payload, metadata, or header:

| Field Pattern | Reason |
|--------------|--------|
| `password` | Plaintext credentials |
| `token` | Authentication tokens |
| `kubeconfig` | Kubernetes cluster access |
| `secret` | Any secret value |
| `private_key` | Cryptographic private keys |
| `certificate_key` | TLS certificate private keys |
| `access_key` | Cloud provider access keys |
| `api_key` | API authentication keys |
| `refresh_token` | OAuth refresh tokens |

### Enforcement

```javascript
const FORBIDDEN_PATTERNS = [
  /password/i,
  /token/i,
  /kubeconfig/i,
  /secret/i,
  /private_key/i,
  /certificate_key/i,
  /access_key/i,
  /api_key/i,
  /refresh_token/i,
];

function validateNoForbiddenFields(payload, path = '') {
  if (typeof payload === 'string') {
    for (const pattern of FORBIDDEN_PATTERNS) {
      if (pattern.test(payload)) {
        throw new Error(`Forbidden field pattern detected at ${path}`);
      }
    }
  }
  if (payload && typeof payload === 'object') {
    for (const [key, value] of Object.entries(payload)) {
      for (const pattern of FORBIDDEN_PATTERNS) {
        if (pattern.test(key)) {
          throw new Error(`Forbidden field name: ${path}.${key}`);
        }
      }
      validateNoForbiddenFields(value, `${path}.${key}`);
    }
  }
}
```

## Payload Reference Gate

When payload exceeds size limits or contains large binary data:

1. Store payload in OCI/S3 blob store
2. Set `payloadRef` in Envelope to immutable reference (digest-based)
3. Set `payload` to `null` or omit
4. Consumer fetches payload via `payloadRef` if needed
5. Payload Reference must expire after max retention period

### Payload Reference Format

```
oci://registry.hnb.cloud/artifacts/messages/<sha256>:<digest>
s3://hnb-messages/<tenant>/<message-id>
```

## Security Validation

The following checks are performed on every message before publish:

1. **Size check**: Total message size <= per-type limit
2. **Forbidden field scan**: Recursive property name and value check
3. **Payload reference validation**: If `payloadRef` is set, verify it points to an allowed storage backend
4. **Schema validation**: Payload must conform to the message type's JSON Schema
5. **Secret reference check**: Only `SecretReference` objects are allowed for credential references, never inline values