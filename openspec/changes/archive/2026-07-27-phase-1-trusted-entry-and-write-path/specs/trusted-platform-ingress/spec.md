## ADDED Requirements

### Requirement: [P1-ING-001] Unified Verified Identity Contract
Every protected platform entry point SHALL accept only a versioned identity
credential whose signature, algorithm, issuer, audience, expiry, not-before,
key identifier, subject, and subject type are verified against the same
approved claim profile. Verification failure SHALL stop processing before any
domain handler or repository access.

**Traceability:** P0-BASE-004, TENANT-005, TENANT-006

#### Scenario: Token was issued for a different audience
- **GIVEN** a correctly signed token whose audience does not include the HNB platform entry point
- **WHEN** the token is presented to a protected route
- **THEN** the request is rejected before the handler and the reason is recorded without logging the token

### Requirement: [P1-ING-002] Trusted Context Derivation and Header Sanitization
The trusted ingress SHALL derive subject and tenant context from verified
claims and authorized membership, SHALL remove or overwrite inbound identity
headers, and SHALL propagate a typed context to downstream code. A caller
supplied tenant, user, role, permission, or approval value SHALL NOT become
authoritative identity.

**Traceability:** P0-BASE-004, TENANT-005

#### Scenario: Caller spoofs tenant and user headers
- **GIVEN** a valid tenant-A token and caller-supplied headers naming tenant B and another user
- **WHEN** the request crosses trusted ingress
- **THEN** downstream code receives only the verified tenant-A subject context and the spoofed values cannot influence authorization

### Requirement: [P1-ING-003] Scope-Aware Fail-Closed Authorization
Every protected operation SHALL be authorized using subject, tenant,
resource kind, resource identifier, action, and applicable
project/environment/namespace scope. Missing policy data, unavailable policy
evaluation, or scope mismatch SHALL deny the operation, and repositories SHALL
apply the same tenant boundary used by the decision.

**Traceability:** TENANT-006, TENANT-007, P0-BASE-004

#### Scenario: Subject has permission in a different namespace
- **GIVEN** a subject may read pods in namespace A but has no grant in namespace B
- **WHEN** the subject requests a pod in namespace B
- **THEN** authorization denies the request and no namespace-B repository or target query is performed

### Requirement: [P1-ING-004] Audience-Restricted Service Identity
Service-to-service calls SHALL use an authenticated workload or service
identity restricted to the intended audience and action. Services SHALL NOT
trust network location, shared caller-controlled headers, or unrestricted
replayed end-user tokens as service authentication.

**Traceability:** TENANT-005, CONTRACT-001, P0-BASE-004

#### Scenario: Internal request lacks a service audience
- **GIVEN** a request reaches an internal intent endpoint with a user token not issued for that service
- **WHEN** the service authenticates the caller
- **THEN** it rejects the request and creates no intent or Operation

### Requirement: [P1-ING-005] Key Rotation and Credential Revocation
Identity signing and verification keys SHALL have versioned identifiers,
bounded overlap, protected private material, and tested rotation and emergency
revocation procedures. Disabled subjects and revoked credentials SHALL cease
authorizing new requests within the documented propagation bound.

**Traceability:** TENANT-008, OBS-004, P0-BASE-004

#### Scenario: Signing key is emergency-revoked
- **GIVEN** a previously valid token signed by a compromised key
- **WHEN** the key is marked revoked and the maximum propagation bound elapses
- **THEN** every protected entry point rejects that token while accepting tokens signed by active keys

### Requirement: [P1-ING-006] Security Audit and Correlation
Security-sensitive actions SHALL emit tenant-scoped audit evidence for
authentication, tenant selection, authorization, intent submission, approval,
denial, cancellation, and credential lifecycle actions with subject, decision, policy/key version,
resource/action, correlation ID, and outcome. Evidence SHALL redact tokens,
Secrets, credentials, and sensitive request values.

**Traceability:** OP-006, OBS-001, SEC-005

#### Scenario: Auditor traces a denied runtime mutation
- **GIVEN** a subject attempts a runtime mutation without the required scoped permission
- **WHEN** authorization denies the request
- **THEN** audit evidence identifies the verified subject, scope, action, policy version, correlation, and denial outcome without sensitive values
