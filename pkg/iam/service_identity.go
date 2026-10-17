package iam

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	ServiceIdentityStatusActive  = "active"
	ServiceIdentityStatusRevoked = "revoked"

	DefaultServiceCredentialTTL = 60 * time.Second
	MaxServiceCredentialTTL     = MaxAccessTokenTTL
	DefaultEnrollmentTTL        = 15 * time.Minute
	MaxEnrollmentTTL            = 15 * time.Minute
)

var (
	ErrServiceIdentityNotFound        = errors.New("service identity not found")
	ErrServiceIdentityExists          = errors.New("service identity already exists")
	ErrServiceIdentityVersionConflict = errors.New("service identity version conflict")
	ErrServiceCredentialNotFound      = errors.New("service credential not found")
	ErrServiceCredentialConflict      = errors.New("service credential conflict")
	ErrEnrollmentUnavailable          = errors.New("enrollment credential unavailable")
)

// ServiceIdentityRecord is one exact (tenant, subject, audience, permission)
// registration. It is not a user membership and does not carry a credential.
type ServiceIdentityRecord struct {
	ID                 string
	TenantID           string
	SubjectID          string
	Audience           string
	Permission         ScopedPermission
	Status             string
	Version            int64
	CreatedBySubjectID string
	RevokedBySubjectID string
	RevocationReason   string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	RevokedAt          *time.Time
}

// ServiceCredentialRecord is the issuance/revocation metadata for one signed
// short-lived credential. The credential value itself is never stored.
type ServiceCredentialRecord struct {
	ID                string
	ServiceIdentityID string
	TenantID          string
	SubjectID         string
	Audience          string
	IssuedAt          time.Time
	ExpiresAt         time.Time
	RevokedAt         *time.Time
}

type ServiceCredentialStatus struct {
	Credential ServiceCredentialRecord
	Identity   ServiceIdentityRecord
}

// EnrollmentCredentialRecord is a hashed, single-use bootstrap credential. The
// plaintext token is returned once to the caller and never persisted.
type EnrollmentCredentialRecord struct {
	ID                 string
	TokenHash          string
	TenantID           string
	TargetID           string
	ExpectedSubject    string
	IssuedBySubjectID  string
	ExpiresAt          time.Time
	ConsumedAt         *time.Time
	ConsumedBySubject  string
	RevokedAt          *time.Time
	RevokedBySubjectID string
	CreatedAt          time.Time
}

// ServiceIdentityStore is the owning-plane persistence boundary for service
// identities, credential issuance/revocation, and one-time enrollment.
type ServiceIdentityStore interface {
	CreateServiceIdentity(context.Context, ServiceIdentityRecord) (*ServiceIdentityRecord, error)
	GetServiceIdentity(context.Context, string, string) (*ServiceIdentityRecord, error)
	ListServiceIdentities(context.Context, string) ([]ServiceIdentityRecord, error)
	RegisterServiceCredential(context.Context, ServiceCredentialRecord) error
	GetServiceCredentialStatus(context.Context, string, string) (*ServiceCredentialStatus, error)
	RevokeServiceIdentity(context.Context, string, string, int64, string, string, time.Time) (*ServiceIdentityRecord, error)
	RevokeServiceCredential(context.Context, string, string, string, string, string, time.Time) error
	CreateEnrollmentCredential(context.Context, EnrollmentCredentialRecord) error
	ConsumeEnrollmentCredential(context.Context, string, string, string, string, string, time.Time) (*EnrollmentCredentialRecord, error)
	RevokeEnrollmentCredential(context.Context, string, string, string, string, time.Time) error
}

type ServiceIdentityDBStore struct {
	db *sql.DB
}

func NewServiceIdentityDBStore(db *sql.DB) *ServiceIdentityDBStore {
	return &ServiceIdentityDBStore{db: db}
}

var _ ServiceIdentityStore = (*ServiceIdentityDBStore)(nil)

const serviceIdentityColumns = `
	id::text, tenant_id, subject_id::text, audience, permission, status, version,
	COALESCE(created_by_subject_id::text, ''), COALESCE(revoked_by_subject_id::text, ''),
	COALESCE(revocation_reason, ''), created_at, updated_at, revoked_at`

func scanServiceIdentity(scanner interface{ Scan(...any) error }) (*ServiceIdentityRecord, error) {
	var record ServiceIdentityRecord
	var permission []byte
	var revokedAt sql.NullTime
	if err := scanner.Scan(
		&record.ID, &record.TenantID, &record.SubjectID, &record.Audience, &permission,
		&record.Status, &record.Version, &record.CreatedBySubjectID, &record.RevokedBySubjectID,
		&record.RevocationReason, &record.CreatedAt, &record.UpdatedAt, &revokedAt,
	); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(permission, &record.Permission); err != nil {
		return nil, fmt.Errorf("decode service identity permission: %w", err)
	}
	if revokedAt.Valid {
		value := revokedAt.Time
		record.RevokedAt = &value
	}
	return &record, nil
}

func (s *ServiceIdentityDBStore) CreateServiceIdentity(ctx context.Context, record ServiceIdentityRecord) (*ServiceIdentityRecord, error) {
	permission, err := json.Marshal(record.Permission)
	if err != nil {
		return nil, err
	}
	row := s.db.QueryRowContext(ctx, `
		INSERT INTO service_identities (
			id, tenant_id, subject_id, audience, permission, status, version, created_by_subject_id, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, 'active', 1, NULLIF($6, '')::uuid, $7, $7)
		ON CONFLICT DO NOTHING
		RETURNING `+serviceIdentityColumns,
		record.ID, record.TenantID, record.SubjectID, record.Audience, permission, record.CreatedBySubjectID, record.CreatedAt)
	created, err := scanServiceIdentity(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrServiceIdentityExists
	}
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (s *ServiceIdentityDBStore) GetServiceIdentity(ctx context.Context, tenantID, id string) (*ServiceIdentityRecord, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+serviceIdentityColumns+`
		FROM service_identities WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	record, err := scanServiceIdentity(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrServiceIdentityNotFound
	}
	if err != nil {
		return nil, err
	}
	return record, nil
}

func (s *ServiceIdentityDBStore) ListServiceIdentities(ctx context.Context, tenantID string) ([]ServiceIdentityRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+serviceIdentityColumns+`
		FROM service_identities WHERE tenant_id = $1 ORDER BY created_at, id`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []ServiceIdentityRecord
	for rows.Next() {
		record, err := scanServiceIdentity(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, *record)
	}
	return records, rows.Err()
}

func (s *ServiceIdentityDBStore) RegisterServiceCredential(ctx context.Context, record ServiceCredentialRecord) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO service_credentials (
			id, service_identity_id, tenant_id, subject_id, audience, issued_at, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		record.ID, record.ServiceIdentityID, record.TenantID, record.SubjectID, record.Audience, record.IssuedAt, record.ExpiresAt)
	return err
}

func (s *ServiceIdentityDBStore) GetServiceCredentialStatus(ctx context.Context, tenantID, credentialID string) (*ServiceCredentialStatus, error) {
	var status ServiceCredentialStatus
	var permission []byte
	var revokedAt sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT c.id::text, c.service_identity_id::text, c.tenant_id, c.subject_id::text, c.audience,
		       c.issued_at, c.expires_at, c.revoked_at,
		       i.status, i.version, i.permission
		FROM service_credentials c
		JOIN service_identities i ON i.id = c.service_identity_id AND i.tenant_id = c.tenant_id
		WHERE c.id = $1 AND c.tenant_id = $2`, credentialID, tenantID).
		Scan(&status.Credential.ID, &status.Credential.ServiceIdentityID, &status.Credential.TenantID,
			&status.Credential.SubjectID, &status.Credential.Audience, &status.Credential.IssuedAt,
			&status.Credential.ExpiresAt, &revokedAt, &status.Identity.Status, &status.Identity.Version, &permission)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrServiceCredentialNotFound
	}
	if err != nil {
		return nil, err
	}
	if revokedAt.Valid {
		value := revokedAt.Time
		status.Credential.RevokedAt = &value
	}
	if err := json.Unmarshal(permission, &status.Identity.Permission); err != nil {
		return nil, fmt.Errorf("decode service identity permission: %w", err)
	}
	status.Identity.ID = status.Credential.ServiceIdentityID
	status.Identity.TenantID = status.Credential.TenantID
	status.Identity.SubjectID = status.Credential.SubjectID
	status.Identity.Audience = status.Credential.Audience
	return &status, nil
}

func (s *ServiceIdentityDBStore) RevokeServiceIdentity(ctx context.Context, tenantID, id string, expectedVersion int64, actorSubjectID, reason string, now time.Time) (*ServiceIdentityRecord, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		UPDATE service_identities
		SET status = 'revoked', version = version + 1, revoked_at = $4,
		    revoked_by_subject_id = NULLIF($5, '')::uuid, revocation_reason = NULLIF($6, ''),
		    updated_at = $4
		WHERE id = $1 AND tenant_id = $2 AND status = 'active' AND version = $3`,
		id, tenantID, expectedVersion, now, actorSubjectID, reason)
	if err != nil {
		return nil, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		var status string
		var version int64
		err := tx.QueryRowContext(ctx, `
			SELECT status, version FROM service_identities WHERE id = $1 AND tenant_id = $2`, id, tenantID).
			Scan(&status, &version)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrServiceIdentityNotFound
		}
		if err != nil {
			return nil, err
		}
		if status == ServiceIdentityStatusRevoked {
			return nil, ErrServiceIdentityRevoked
		}
		return nil, ErrServiceIdentityVersionConflict
	}
	row := tx.QueryRowContext(ctx, `SELECT `+serviceIdentityColumns+`
		FROM service_identities WHERE id = $1 AND tenant_id = $2`, id, tenantID)
	record, err := scanServiceIdentity(row)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return record, nil
}

func (s *ServiceIdentityDBStore) RevokeServiceCredential(ctx context.Context, tenantID, identityID, credentialID, actorSubjectID, reason string, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	result, err := tx.ExecContext(ctx, `
		UPDATE service_credentials
		SET revoked_at = $4, revoked_by_subject_id = NULLIF($5, '')::uuid,
		    revocation_reason = NULLIF($6, '')
		WHERE id = $1 AND tenant_id = $2 AND service_identity_id = $3 AND revoked_at IS NULL`,
		credentialID, tenantID, identityID, now, actorSubjectID, reason)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		var revokedAt sql.NullTime
		err := tx.QueryRowContext(ctx, `
			SELECT revoked_at FROM service_credentials
			WHERE id = $1 AND tenant_id = $2 AND service_identity_id = $3`,
			credentialID, tenantID, identityID).Scan(&revokedAt)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrServiceCredentialNotFound
		}
		if err != nil {
			return err
		}
		if revokedAt.Valid {
			return tx.Commit()
		}
		return ErrServiceCredentialConflict
	}
	return tx.Commit()
}

func (s *ServiceIdentityDBStore) CreateEnrollmentCredential(ctx context.Context, record EnrollmentCredentialRecord) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO service_enrollment_credentials (
			id, token_hash, tenant_id, target_id, expected_subject, issued_by_subject_id, expires_at, created_at
		) VALUES ($1, $2, $3, $4, $5, NULLIF($6, '')::uuid, $7, $8)`,
		record.ID, record.TokenHash, record.TenantID, record.TargetID, record.ExpectedSubject,
		record.IssuedBySubjectID, record.ExpiresAt, record.CreatedAt)
	return err
}

// ConsumeEnrollmentCredential is the single atomic consumption point. The
// UPDATE predicate is the authorization boundary: only one caller can move an
// unexpired, unconsumed credential for the exact (tenant, target, subject) to
// consumed, and every losing caller receives ErrEnrollmentUnavailable.
func (s *ServiceIdentityDBStore) ConsumeEnrollmentCredential(ctx context.Context, tokenHash, tenantID, targetID, expectedSubject, consumedBySubject string, now time.Time) (*EnrollmentCredentialRecord, error) {
	var record EnrollmentCredentialRecord
	var consumedAt, revokedAt sql.NullTime
	var issuedBy sql.NullString
	err := s.db.QueryRowContext(ctx, `
		UPDATE service_enrollment_credentials
		SET consumed_at = $5, consumed_by_subject = $6
		WHERE token_hash = $1 AND tenant_id = $2 AND target_id = $3 AND expected_subject = $4
		  AND consumed_at IS NULL AND revoked_at IS NULL AND expires_at > $5
		RETURNING id::text, token_hash, tenant_id, target_id, expected_subject,
		          COALESCE(issued_by_subject_id::text, ''), expires_at, consumed_at,
		          COALESCE(consumed_by_subject, ''), revoked_at, created_at`,
		tokenHash, tenantID, targetID, expectedSubject, now, consumedBySubject).
		Scan(&record.ID, &record.TokenHash, &record.TenantID, &record.TargetID, &record.ExpectedSubject,
			&issuedBy, &record.ExpiresAt, &consumedAt, &record.ConsumedBySubject, &revokedAt, &record.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrEnrollmentUnavailable
	}
	if err != nil {
		return nil, err
	}
	record.IssuedBySubjectID = issuedBy.String
	if consumedAt.Valid {
		value := consumedAt.Time
		record.ConsumedAt = &value
	}
	if revokedAt.Valid {
		value := revokedAt.Time
		record.RevokedAt = &value
	}
	return &record, nil
}

func (s *ServiceIdentityDBStore) RevokeEnrollmentCredential(ctx context.Context, tenantID, targetID, enrollmentID, actorSubjectID string, now time.Time) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE service_enrollment_credentials
		SET revoked_at = $5, revoked_by_subject_id = NULLIF($4, '')::uuid
		WHERE id = $1 AND tenant_id = $2 AND target_id = $3
		  AND consumed_at IS NULL AND revoked_at IS NULL`,
		enrollmentID, tenantID, targetID, actorSubjectID, now)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrEnrollmentUnavailable
	}
	return nil
}

// AgentSessionRecord is one renewable Agent tunnel session. Sessions are
// separate from the single-use enrollment credential: rotation keeps the same
// session ID and generation, while revocation makes every future renewal fail.
type AgentSessionRecord struct {
	ID         string
	TenantID   string
	TargetID   string
	Subject    string
	Generation int64
	IssuedAt   time.Time
	ExpiresAt  time.Time
	RevokedAt  *time.Time
}

type AgentSessionStore interface {
	CreateAgentSession(context.Context, AgentSessionRecord) (*AgentSessionRecord, error)
	GetAgentSession(context.Context, string, string, string) (*AgentSessionRecord, error)
	RotateAgentSession(context.Context, string, string, string, time.Time, time.Time) (*AgentSessionRecord, error)
	RevokeAgentSessions(context.Context, string, string, string, time.Time) (int64, error)
}

var (
	ErrAgentSessionNotFound    = errors.New("agent session not found")
	ErrAgentSessionUnavailable = errors.New("agent session unavailable")
)

const agentSessionColumns = `
	id::text, tenant_id, target_id, subject, generation, issued_at, expires_at, revoked_at`

func scanAgentSession(scanner interface{ Scan(...any) error }) (*AgentSessionRecord, error) {
	var record AgentSessionRecord
	var revokedAt sql.NullTime
	if err := scanner.Scan(&record.ID, &record.TenantID, &record.TargetID, &record.Subject,
		&record.Generation, &record.IssuedAt, &record.ExpiresAt, &revokedAt); err != nil {
		return nil, err
	}
	if revokedAt.Valid {
		value := revokedAt.Time
		record.RevokedAt = &value
	}
	return &record, nil
}

func (s *ServiceIdentityDBStore) CreateAgentSession(ctx context.Context, record AgentSessionRecord) (*AgentSessionRecord, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	row := tx.QueryRowContext(ctx, `
		INSERT INTO agent_sessions (
			id, tenant_id, target_id, subject, generation, issued_at, expires_at, created_at
		) VALUES (
			$1, $2, $3, $4,
			COALESCE((SELECT MAX(generation) FROM agent_sessions WHERE tenant_id = $2 AND target_id = $3), 0) + 1,
			$5, $6, $5
		) RETURNING `+agentSessionColumns,
		record.ID, record.TenantID, record.TargetID, record.Subject, record.IssuedAt, record.ExpiresAt)
	created, err := scanAgentSession(row)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return created, nil
}

func (s *ServiceIdentityDBStore) GetAgentSession(ctx context.Context, tenantID, targetID, sessionID string) (*AgentSessionRecord, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+agentSessionColumns+`
		FROM agent_sessions WHERE id = $1 AND tenant_id = $2 AND target_id = $3`, sessionID, tenantID, targetID)
	record, err := scanAgentSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAgentSessionNotFound
	}
	if err != nil {
		return nil, err
	}
	return record, nil
}

func (s *ServiceIdentityDBStore) RotateAgentSession(ctx context.Context, tenantID, targetID, sessionID string, issuedAt, expiresAt time.Time) (*AgentSessionRecord, error) {
	row := s.db.QueryRowContext(ctx, `
		UPDATE agent_sessions
		SET issued_at = $4, expires_at = $5
		WHERE id = $1 AND tenant_id = $2 AND target_id = $3 AND revoked_at IS NULL
		RETURNING `+agentSessionColumns,
		sessionID, tenantID, targetID, issuedAt, expiresAt)
	record, err := scanAgentSession(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAgentSessionUnavailable
	}
	if err != nil {
		return nil, err
	}
	return record, nil
}

func (s *ServiceIdentityDBStore) RevokeAgentSessions(ctx context.Context, tenantID, targetID, actorSubjectID string, now time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE agent_sessions
		SET revoked_at = $3, revoked_by_subject_id = NULLIF($4, '')::uuid
		WHERE tenant_id = $1 AND target_id = $2 AND revoked_at IS NULL`,
		tenantID, targetID, now, actorSubjectID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
