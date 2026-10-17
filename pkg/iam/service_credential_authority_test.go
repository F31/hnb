package iam

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

type fakeServiceIdentityStore struct {
	mu          sync.Mutex
	identities  map[string]ServiceIdentityRecord
	credentials map[string]ServiceCredentialStatus
	enrollments map[string]EnrollmentCredentialRecord
}

func newFakeServiceIdentityStore() *fakeServiceIdentityStore {
	return &fakeServiceIdentityStore{
		identities:  make(map[string]ServiceIdentityRecord),
		credentials: make(map[string]ServiceCredentialStatus),
		enrollments: make(map[string]EnrollmentCredentialRecord),
	}
}

func serviceIdentityKey(tenantID, id string) string { return tenantID + "|" + id }
func serviceCredentialKey(tenantID, id string) string {
	return tenantID + "|" + id
}

func (s *fakeServiceIdentityStore) CreateServiceIdentity(_ context.Context, record ServiceIdentityRecord) (*ServiceIdentityRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.identities {
		if existing.TenantID == record.TenantID && existing.SubjectID == record.SubjectID && existing.Audience == record.Audience {
			return nil, ErrServiceIdentityExists
		}
	}
	s.identities[serviceIdentityKey(record.TenantID, record.ID)] = record
	copy := record
	return &copy, nil
}

func (s *fakeServiceIdentityStore) GetServiceIdentity(_ context.Context, tenantID, id string) (*ServiceIdentityRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.identities[serviceIdentityKey(tenantID, id)]
	if !ok {
		return nil, ErrServiceIdentityNotFound
	}
	copy := record
	return &copy, nil
}

func (s *fakeServiceIdentityStore) ListServiceIdentities(_ context.Context, tenantID string) ([]ServiceIdentityRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var records []ServiceIdentityRecord
	for _, record := range s.identities {
		if record.TenantID == tenantID {
			records = append(records, record)
		}
	}
	return records, nil
}

func (s *fakeServiceIdentityStore) RegisterServiceCredential(_ context.Context, record ServiceCredentialRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	identity, ok := s.identities[serviceIdentityKey(record.TenantID, record.ServiceIdentityID)]
	if !ok {
		return ErrServiceIdentityNotFound
	}
	s.credentials[serviceCredentialKey(record.TenantID, record.ID)] = ServiceCredentialStatus{
		Credential: record, Identity: identity,
	}
	return nil
}

func (s *fakeServiceIdentityStore) GetServiceCredentialStatus(_ context.Context, tenantID, credentialID string) (*ServiceCredentialStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	status, ok := s.credentials[serviceCredentialKey(tenantID, credentialID)]
	if !ok {
		return nil, ErrServiceCredentialNotFound
	}
	if identity, ok := s.identities[serviceIdentityKey(status.Credential.TenantID, status.Credential.ServiceIdentityID)]; ok {
		status.Identity = identity
	}
	return &status, nil
}

func (s *fakeServiceIdentityStore) RevokeServiceIdentity(_ context.Context, tenantID, id string, expectedVersion int64, actorSubjectID, reason string, now time.Time) (*ServiceIdentityRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.identities[serviceIdentityKey(tenantID, id)]
	if !ok {
		return nil, ErrServiceIdentityNotFound
	}
	if record.Status == ServiceIdentityStatusRevoked {
		return nil, ErrServiceIdentityRevoked
	}
	if record.Version != expectedVersion {
		return nil, ErrServiceIdentityVersionConflict
	}
	record.Status = ServiceIdentityStatusRevoked
	record.Version++
	record.RevokedAt = &now
	record.RevokedBySubjectID = actorSubjectID
	record.RevocationReason = reason
	record.UpdatedAt = now
	s.identities[serviceIdentityKey(tenantID, id)] = record
	copy := record
	return &copy, nil
}

func (s *fakeServiceIdentityStore) RevokeServiceCredential(_ context.Context, tenantID, identityID, credentialID, actorSubjectID, reason string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := serviceCredentialKey(tenantID, credentialID)
	status, ok := s.credentials[key]
	if !ok || status.Credential.ServiceIdentityID != identityID {
		return ErrServiceCredentialNotFound
	}
	if status.Credential.RevokedAt != nil {
		return nil
	}
	status.Credential.RevokedAt = &now
	s.credentials[key] = status
	return nil
}

func (s *fakeServiceIdentityStore) CreateEnrollmentCredential(_ context.Context, record EnrollmentCredentialRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.enrollments[record.TokenHash]; exists {
		return ErrEnrollmentUnavailable
	}
	s.enrollments[record.TokenHash] = record
	return nil
}

func (s *fakeServiceIdentityStore) ConsumeEnrollmentCredential(_ context.Context, tokenHash, tenantID, targetID, expectedSubject, consumedBySubject string, now time.Time) (*EnrollmentCredentialRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.enrollments[tokenHash]
	if !ok || record.TenantID != tenantID || record.TargetID != targetID || record.ExpectedSubject != expectedSubject ||
		record.ConsumedAt != nil || record.RevokedAt != nil || !record.ExpiresAt.After(now) {
		return nil, ErrEnrollmentUnavailable
	}
	record.ConsumedAt = &now
	record.ConsumedBySubject = consumedBySubject
	s.enrollments[tokenHash] = record
	return &record, nil
}

func (s *fakeServiceIdentityStore) RevokeEnrollmentCredential(_ context.Context, tenantID, targetID, enrollmentID, actorSubjectID string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for hash, record := range s.enrollments {
		if record.ID == enrollmentID && record.TenantID == tenantID && record.TargetID == targetID {
			if record.ConsumedAt != nil || record.RevokedAt != nil {
				return ErrEnrollmentUnavailable
			}
			record.RevokedAt = &now
			record.RevokedBySubjectID = actorSubjectID
			s.enrollments[hash] = record
			return nil
		}
	}
	return ErrEnrollmentUnavailable
}

type auditRecorder struct {
	mu     sync.Mutex
	events []ServiceCredentialAuditEvent
}

func (r *auditRecorder) Record(_ context.Context, event ServiceCredentialAuditEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
}

func newAuthorityForTest(t *testing.T) (*ServiceCredentialAuthority, *fakeServiceIdentityStore, *auditRecorder, testKeys, *time.Time) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keys := testKeys{kid: "test-key-1", key: key}
	now := time.Unix(2_000_000_000, 0).UTC()
	store := newFakeServiceIdentityStore()
	audit := &auditRecorder{}
	authority, err := NewServiceCredentialAuthority(store, keys, keys, "https://issuer.example", func() time.Time { return now }, audit.Record)
	if err != nil {
		t.Fatal(err)
	}
	return authority, store, audit, keys, &now
}

func newServiceIdentityForTest(t *testing.T, authority *ServiceCredentialAuthority, tenantID, audience string) *ServiceIdentityRecord {
	t.Helper()
	subjectID := uuid.NewString()
	actor := TrustedContext{SubjectID: uuid.NewString(), SubjectType: "user", TenantID: tenantID, PolicyVersion: "default:1"}
	identity, err := authority.CreateIdentity(context.Background(), actor, ServiceIdentityInput{
		TenantID: tenantID, SubjectID: subjectID, Audience: audience,
		Permission: ScopedPermission{ResourceKind: "runtimeProvider", ResourceID: "provider-a", Action: ActionExecute, TenantID: tenantID},
	})
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func TestServiceCredentialAuthorityIssueRenewRevoke(t *testing.T) {
	authority, _, audit, _, now := newAuthorityForTest(t)
	identity := newServiceIdentityForTest(t, authority, "tenant-a", "provider-audience")
	actor := TrustedContext{SubjectID: uuid.NewString(), SubjectType: "user", TenantID: "tenant-a"}

	first, err := authority.Issue(context.Background(), actor, identity.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if first.ExpiresAt.Sub(*now) > DefaultServiceCredentialTTL {
		t.Fatalf("default TTL exceeded: %v", first.ExpiresAt.Sub(*now))
	}
	verified, err := authority.Verify(context.Background(), first.Credential)
	if err != nil {
		t.Fatal(err)
	}
	if verified.Claims.SubjectID != identity.SubjectID || verified.Claims.Audiences[0] != identity.Audience ||
		verified.Claims.ScopedPermissions[0] != identity.Permission {
		t.Fatalf("unexpected verified credential: %+v", verified.Claims)
	}

	second, err := authority.Renew(context.Background(), first.Credential)
	if err != nil {
		t.Fatal(err)
	}
	if second.CredentialID == first.CredentialID {
		t.Fatal("renewal returned the same credential ID")
	}
	if _, err := authority.Verify(context.Background(), second.Credential); err != nil {
		t.Fatal(err)
	}

	if err := authority.RevokeCredential(context.Background(), actor, identity.ID, first.CredentialID, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := authority.Verify(context.Background(), first.Credential); !errors.Is(err, ErrServiceCredentialRejected) {
		t.Fatalf("revoked credential was accepted: %v", err)
	}
	if _, err := authority.Verify(context.Background(), second.Credential); err != nil {
		t.Fatalf("unrelated credential was rejected: %v", err)
	}

	if _, err := authority.RevokeIdentity(context.Background(), actor, identity.ID, 1, "retired"); err != nil {
		t.Fatal(err)
	}
	if _, err := authority.Verify(context.Background(), second.Credential); !errors.Is(err, ErrServiceIdentityRevoked) {
		t.Fatalf("credential of revoked identity was accepted: %v", err)
	}
	if _, err := authority.Issue(context.Background(), actor, identity.ID, 0); !errors.Is(err, ErrServiceIdentityRevoked) {
		t.Fatalf("issuance after revocation was accepted: %v", err)
	}

	if len(audit.events) < 4 {
		t.Fatalf("expected issue/renew/revoke audit events, got %+v", audit.events)
	}
	for _, event := range audit.events {
		if event.TenantID != "tenant-a" || event.ServiceSubjectID == "" {
			t.Fatalf("audit event lost tenant or service subject: %+v", event)
		}
	}
}

func TestServiceCredentialAuthorityRejectsCrossTenantAndNonUserRevocation(t *testing.T) {
	authority, _, _, _, _ := newAuthorityForTest(t)
	identity := newServiceIdentityForTest(t, authority, "tenant-a", "provider-audience")

	crossTenant := TrustedContext{SubjectID: uuid.NewString(), SubjectType: "user", TenantID: "tenant-b"}
	if _, err := authority.Issue(context.Background(), crossTenant, identity.ID, 0); !errors.Is(err, ErrServiceIdentityNotFound) {
		t.Fatalf("cross-tenant issuance was accepted: %v", err)
	}
	if _, err := authority.RevokeIdentity(context.Background(), crossTenant, identity.ID, 1, "x"); !errors.Is(err, ErrServiceIdentityNotFound) {
		t.Fatalf("cross-tenant revocation was accepted: %v", err)
	}

	serviceActor := TrustedContext{SubjectID: uuid.NewString(), SubjectType: "service", TenantID: "tenant-a"}
	if _, err := authority.RevokeIdentity(context.Background(), serviceActor, identity.ID, 1, "x"); !errors.Is(err, ErrServiceCredentialRejected) {
		t.Fatalf("service actor revoked an identity: %v", err)
	}
	if _, err := authority.Issue(context.Background(), serviceActor, identity.ID, 0); !errors.Is(err, ErrServiceCredentialRejected) {
		t.Fatalf("service actor issued for another subject: %v", err)
	}

	selfActor := TrustedContext{SubjectID: identity.SubjectID, SubjectType: "service", TenantID: "tenant-a"}
	if _, err := authority.Issue(context.Background(), selfActor, identity.ID, 0); err != nil {
		t.Fatalf("self issuance failed: %v", err)
	}
}

func TestServiceCredentialAuthorityRejectsVersionConflictTamperAndExpiry(t *testing.T) {
	authority, store, _, _, now := newAuthorityForTest(t)
	identity := newServiceIdentityForTest(t, authority, "tenant-a", "provider-audience")
	actor := TrustedContext{SubjectID: uuid.NewString(), SubjectType: "user", TenantID: "tenant-a"}

	issued, err := authority.Issue(context.Background(), actor, identity.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(issued.Credential, ".")
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	payload[0] ^= 1
	parts[1] = base64.RawURLEncoding.EncodeToString(payload)
	if _, err := authority.Verify(context.Background(), strings.Join(parts, ".")); !errors.Is(err, ErrServiceCredentialRejected) {
		t.Fatal("tampered credential was accepted")
	}

	if _, err := authority.RevokeIdentity(context.Background(), actor, identity.ID, 9, "x"); !errors.Is(err, ErrServiceIdentityVersionConflict) {
		t.Fatalf("wrong version was accepted: %v", err)
	}

	*now = now.Add(MaxServiceCredentialTTL + time.Second)
	if _, err := authority.Verify(context.Background(), issued.Credential); !errors.Is(err, ErrServiceCredentialRejected) {
		t.Fatal("expired credential was accepted")
	}

	*now = time.Unix(2_000_000_000, 0).UTC()
	store.mu.Lock()
	record := store.identities[serviceIdentityKey("tenant-a", identity.ID)]
	record.SubjectID = uuid.NewString()
	store.identities[serviceIdentityKey("tenant-a", identity.ID)] = record
	store.mu.Unlock()
	if _, err := authority.Verify(context.Background(), issued.Credential); !errors.Is(err, ErrServiceCredentialRejected) {
		t.Fatal("credential with changed stored subject was accepted")
	}
}
