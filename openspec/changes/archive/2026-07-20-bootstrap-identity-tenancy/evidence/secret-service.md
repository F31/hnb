# SecretReference Service Design Evidence

## Architecture

```
API Request
    |
    v
SecretReference Service
    |
    ├── Create: encrypt value with AES-256-GCM, store in secret_references
    ├── Read: decrypt on demand, return to authorized caller
    ├── Rotate: create new version, update active reference
    ├── Resolve: return SecretReference URI (not decrypted value) for ExecutionPlan
    └── Provider: resolve SecretReference to actual credential at execution time
```

## Encryption

```go
type SecretService struct {
    db          *sql.DB
    masterKey   []byte  // from platform key management service
    cache       *cache.Cache
}

func (s *SecretService) Encrypt(plaintext []byte) (string, error) {
    nonce := make([]byte, 12)
    if _, err := rand.Read(nonce); err != nil {
        return "", err
    }
    ciphertext, err := cipher.AESGCM.Seal(nil, nonce, plaintext, nil)
    if err != nil {
        return "", err
    }
    // Store as base64(nonce + ciphertext)
    return base64.StdEncoding.EncodeToString(append(nonce, ciphertext...)), nil
}

func (s *SecretService) Decrypt(encoded string) ([]byte, error) {
    data, err := base64.StdEncoding.DecodeString(encoded)
    if err != nil {
        return nil, err
    }
    nonce, ciphertext := data[:12], data[12:]
    plaintext, err := cipher.AESGCM.Open(nil, nonce, ciphertext, nil)
    if err != nil {
        return nil, err
    }
    return plaintext, nil
}
```

## SecretReference Format

```
secret://tenant/{tenant_id}/{secret_name}
```

Example: `secret://tenant/t1/db-password`

## Rotation Policy

```json
{
  "interval": "30d",
  "autoRotate": true,
  "lastRotatedAt": "2026-07-01T00:00:00Z",
  "nextRotationAt": "2026-07-31T00:00:00Z"
}
```

## Provider Resolution Flow

1. ExecutionPlan contains `secret://tenant/t1/db-password`
2. Provider receives the SecretReference in the Operation context
3. Provider calls SecretService.Resolve(secretRef, context)
4. SecretService validates tenant context matches the reference
5. SecretService decrypts the current version
6. Provider uses the credential for the operation
7. Credential is NEVER logged, stored in ExecutionPlan, or returned to API

## Test Plan
- Encrypt/Decrypt: round-trip produces original value
- Invalid key: decryption with wrong key fails
- Version management: rotate creates new version, old version still accessible
- SecretReference format: validates pattern `secret://tenant/{tenant_id}/{secret_name}`
- Cross-tenant: Tenant A cannot resolve Tenant B's secret
- Provider resolution: correct credential returned at execution time
- No leak: credentials never appear in logs, plans, or API responses