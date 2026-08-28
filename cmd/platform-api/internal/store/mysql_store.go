package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ErrMySQLNotImplemented is returned by every MySQL stub store method. Users
// who set DB_DRIVER=mysql see a clear error at the first real operation, not a
// hard crash on startup, so the health check endpoint still works.
var ErrMySQLNotImplemented = fmt.Errorf("mysql store is not yet implemented: %w", ErrDialectNotImplemented)

type mysqlStubStore struct {
	db *sql.DB
}

func newMySQLStubStore(db *sql.DB) *mysqlStubStore {
	return &mysqlStubStore{db: db}
}

func (m *mysqlStubStore) Ping(ctx context.Context) error {
	return m.db.PingContext(ctx)
}

func (m *mysqlStubStore) Ready(ctx context.Context) error {
	return fmt.Errorf("mysql backend is not production-ready: %w", ErrMySQLNotImplemented)
}

func (m *mysqlStubStore) SubmitOperation(ctx context.Context, cmd SubmitCommand) (*Operation, bool, error) {
	return nil, false, ErrMySQLNotImplemented
}
func (m *mysqlStubStore) SubmitIntent(ctx context.Context, cmd IntentSubmitCommand) (*Operation, bool, error) {
	return nil, false, ErrMySQLNotImplemented
}
func (m *mysqlStubStore) ApproveOperation(ctx context.Context, id, tenantID, actorID, reason string) (*Operation, error) {
	return nil, ErrMySQLNotImplemented
}
func (m *mysqlStubStore) RejectOperation(ctx context.Context, id, tenantID, actorID, reason string) (*Operation, error) {
	return nil, ErrMySQLNotImplemented
}
func (m *mysqlStubStore) CancelOperation(ctx context.Context, id, tenantID, actorID, reason string) (*Operation, error) {
	return nil, ErrMySQLNotImplemented
}
func (m *mysqlStubStore) GetOperation(ctx context.Context, id, tenantID string) (*Operation, error) {
	return nil, ErrMySQLNotImplemented
}
func (m *mysqlStubStore) ListOperations(ctx context.Context, q ListQuery) ([]OperationSummary, int, error) {
	return nil, 0, ErrMySQLNotImplemented
}
func (m *mysqlStubStore) GetIntentCommitment(context.Context, string, string, string, string) (*IntentCommitment, error) {
	return nil, ErrMySQLNotImplemented
}
func (m *mysqlStubStore) CreateRuntimeTarget(ctx context.Context, rt *RuntimeTarget) error {
	return ErrMySQLNotImplemented
}
func (m *mysqlStubStore) GetRuntimeTarget(ctx context.Context, id, tenantID string) (*RuntimeTarget, error) {
	return nil, ErrMySQLNotImplemented
}
func (m *mysqlStubStore) ListRuntimeTargets(ctx context.Context, tenantID string) ([]*RuntimeTarget, error) {
	return nil, ErrMySQLNotImplemented
}
func (m *mysqlStubStore) UpdateRuntimeTargetStatus(ctx context.Context, id, tenantID string, status string, observedAt time.Time) error {
	return ErrMySQLNotImplemented
}
func (m *mysqlStubStore) UpdateRuntimeTargetDescription(ctx context.Context, id, tenantID, description string) error {
	return ErrMySQLNotImplemented
}
func (m *mysqlStubStore) DeleteRuntimeTarget(ctx context.Context, id, tenantID string) error {
	return ErrMySQLNotImplemented
}
func (m *mysqlStubStore) CreateCluster(ctx context.Context, req CreateClusterRequest) (*Cluster, error) {
	return nil, ErrMySQLNotImplemented
}
func (m *mysqlStubStore) GetCluster(ctx context.Context, id, tenantID string) (*Cluster, error) {
	return nil, ErrMySQLNotImplemented
}
func (m *mysqlStubStore) ListClusters(ctx context.Context, tenantID string) ([]*Cluster, error) {
	return nil, ErrMySQLNotImplemented
}
func (m *mysqlStubStore) DeleteCluster(ctx context.Context, id, tenantID string) error {
	return ErrMySQLNotImplemented
}
func (m *mysqlStubStore) HeartbeatCluster(ctx context.Context, id, tenantID string) error {
	return ErrMySQLNotImplemented
}
func (m *mysqlStubStore) UpdateCluster(ctx context.Context, id, tenantID string, req UpdateClusterRequest) (*Cluster, error) {
	return nil, ErrMySQLNotImplemented
}
func (m *mysqlStubStore) GetManifest(ctx context.Context, providerID string) (*ProviderManifest, error) {
	return nil, ErrMySQLNotImplemented
}
func (m *mysqlStubStore) SaveManifest(ctx context.Context, manifest *ProviderManifest) error {
	return ErrMySQLNotImplemented
}
func (m *mysqlStubStore) DeleteManifest(ctx context.Context, providerID string) error {
	return ErrMySQLNotImplemented
}
func (m *mysqlStubStore) CheckCompatibility(ctx context.Context, coreVersion, providerID, providerVersion, targetType string) (*CompatibilityEntry, error) {
	return nil, ErrMySQLNotImplemented
}
func (m *mysqlStubStore) SaveCompatibility(ctx context.Context, entry *CompatibilityEntry) error {
	return ErrMySQLNotImplemented
}
func (m *mysqlStubStore) ExpireConformance(ctx context.Context) ([]string, error) {
	return nil, ErrMySQLNotImplemented
}
func (m *mysqlStubStore) CheckProviderConformance(ctx context.Context, providerID string) error {
	return ErrMySQLNotImplemented
}
func (m *mysqlStubStore) UpdateConformanceLevel(ctx context.Context, providerID, level string, expiresAt *time.Time) error {
	return ErrMySQLNotImplemented
}
func (m *mysqlStubStore) ResolveSecretReference(context.Context, string, string, string, string, string) (*SecretReferenceMetadata, error) {
	return nil, ErrMySQLNotImplemented
}
func (m *mysqlStubStore) RegisterSecretReference(context.Context, RegisterSecretReferenceRequest) (*SecretReferenceMetadata, error) {
	return nil, ErrMySQLNotImplemented
}
func (m *mysqlStubStore) LatestKubeConfigEncrypted(context.Context, string) (string, error) {
	return "", ErrMySQLNotImplemented
}
func (m *mysqlStubStore) KubeConfigEncryptedForRef(context.Context, string, string, string) (string, error) {
	return "", ErrMySQLNotImplemented
}
func (m *mysqlStubStore) AppendSecurityAudit(context.Context, SecurityAuditRecord) error {
	return ErrMySQLNotImplemented
}

// Ensure mysqlStubStore implements Store at compile time.
var _ Store = (*mysqlStubStore)(nil)
