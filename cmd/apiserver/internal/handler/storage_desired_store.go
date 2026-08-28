package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
)

var (
	errStorageDesiredNotFound = errors.New("storage desired state not found")
	errStorageVersionConflict = errors.New("storage desired state version conflict")
	errStorageAlreadyExists   = errors.New("storage desired state already exists")
	errStorageInvalidRef      = errors.New("storage desired state reference is invalid")
)

type secretReference struct {
	Provider string `json:"provider"`
	Scope    string `json:"scope"`
	Name     string `json:"name"`
	Version  string `json:"version,omitempty"`
}

type storageBackendInput struct {
	ProviderType          string           `json:"providerType"`
	ProviderSchemaVersion string           `json:"providerSchemaVersion"`
	BackendID             string           `json:"backendId"`
	DisplayName           string           `json:"displayName"`
	Description           string           `json:"description,omitempty"`
	SecretReference       *secretReference `json:"secretReference,omitempty"`
	Attributes            map[string]any   `json:"attributes,omitempty"`
}

type storageBackendRecord struct {
	ID, TenantID, ProviderType, ProviderSchemaVersion, BackendID, DisplayName, Description string
	SecretReference                                                                        *secretReference
	Attributes                                                                             map[string]any
	Version                                                                                int64
	CreatedAt, UpdatedAt                                                                   time.Time
}

type workloadStorageOfferingInput struct {
	BackendID        string              `json:"backendId,omitempty"`
	Name             string              `json:"name"`
	Description      string              `json:"description,omitempty"`
	ConsumptionModel string              `json:"consumptionModel"`
	ServiceMode      string              `json:"serviceMode"`
	AccessModes      []string            `json:"accessModes"`
	VolumeExpansion  string              `json:"volumeExpansion"`
	Snapshots        string              `json:"snapshots"`
	Clones           string              `json:"clones"`
	Topology         map[string][]string `json:"topology,omitempty"`
	ProtectionClass  string              `json:"protectionClass"`
}

type workloadStorageOfferingRecord struct {
	ID, TenantID, BackendID, Name, Description, ServiceMode string
	AccessModes                                             []string
	VolumeExpansion, Snapshots, Clones                      string
	Topology                                                map[string][]string
	ProtectionClass                                         string
	Version                                                 int64
	CreatedAt, UpdatedAt                                    time.Time
}

type storageClassBindingInput struct {
	OfferingVersion             int64               `json:"offeringVersion"`
	TargetID                    string              `json:"targetId"`
	BindingTarget               string              `json:"bindingTarget"`
	StorageClassName            string              `json:"storageClassName"`
	StorageClassUID             string              `json:"storageClassUid"`
	StorageClassResourceVersion string              `json:"storageClassResourceVersion"`
	SyncState                   string              `json:"syncState"`
	IsDefault                   bool                `json:"isDefault"`
	Source                      string              `json:"source"`
	ObservedAt                  time.Time           `json:"observedAt"`
	Freshness                   string              `json:"freshness"`
	Topology                    map[string][]string `json:"topology,omitempty"`
}

type storageClassBindingRecord struct {
	ID, TenantID, OfferingID                                  string
	OfferingVersion                                           int64
	TargetID, StorageClassName, StorageClassUID               string
	StorageClassResourceVersion, SyncState, Source, Freshness string
	IsDefault                                                 bool
	ObservedAt                                                time.Time
	Topology                                                  map[string][]string
	Conditions                                                []map[string]any
	Version                                                   int64
	CreatedAt, UpdatedAt                                      time.Time
}

type storageDesiredStore interface {
	ListBackends(context.Context, string) ([]storageBackendRecord, error)
	GetBackend(context.Context, string, string) (storageBackendRecord, error)
	CreateBackend(context.Context, string, string, storageBackendInput) (storageBackendRecord, error)
	UpdateBackend(context.Context, string, string, int64, storageBackendInput) (storageBackendRecord, error)
	DeleteBackend(context.Context, string, string, int64) error
	ValidateSecretReference(context.Context, string, secretReference) error
	ListOfferings(context.Context, string) ([]workloadStorageOfferingRecord, error)
	GetOffering(context.Context, string, string) (workloadStorageOfferingRecord, error)
	CreateOffering(context.Context, string, string, workloadStorageOfferingInput) (workloadStorageOfferingRecord, error)
	UpdateOffering(context.Context, string, string, int64, workloadStorageOfferingInput) (workloadStorageOfferingRecord, error)
	DeleteOffering(context.Context, string, string, int64) error
	ListBindings(context.Context, string, string) ([]storageClassBindingRecord, error)
	GetBinding(context.Context, string, string) (storageClassBindingRecord, error)
	CreateBinding(context.Context, string, string, string, storageClassBindingInput) (storageClassBindingRecord, error)
	UpdateBinding(context.Context, string, string, int64, storageClassBindingInput) (storageClassBindingRecord, error)
	DeleteBinding(context.Context, string, string, int64) error
}

type postgresStorageDesiredStore struct{ db *sql.DB }

func NewPostgresStorageDesiredStore(db *sql.DB) storageDesiredStore {
	return &postgresStorageDesiredStore{db: db}
}

const backendColumns = `id, tenant_id, provider_type, COALESCE(provider_schema_version,''), backend_id, display_name, description,
	secret_provider, secret_scope, secret_name, secret_version, attributes, version, created_at, updated_at`

func scanBackend(scanner interface{ Scan(...any) error }) (storageBackendRecord, error) {
	var item storageBackendRecord
	var provider, scope, name, version sql.NullString
	var attributes []byte
	err := scanner.Scan(&item.ID, &item.TenantID, &item.ProviderType, &item.ProviderSchemaVersion, &item.BackendID, &item.DisplayName,
		&item.Description, &provider, &scope, &name, &version, &attributes, &item.Version, &item.CreatedAt, &item.UpdatedAt)
	if err == nil {
		err = json.Unmarshal(attributes, &item.Attributes)
	}
	if err == nil && provider.Valid {
		item.SecretReference = &secretReference{Provider: provider.String, Scope: scope.String, Name: name.String, Version: version.String}
	}
	return item, err
}

func (s *postgresStorageDesiredStore) ValidateSecretReference(ctx context.Context, tenantID string, ref secretReference) error {
	var exists bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM secret_references sr
		JOIN kms_providers kp ON kp.id=sr.kms_provider_id AND kp.is_active
		WHERE sr.tenant_id=$1 AND kp.name=$2 AND sr.scope=$3 AND sr.name=$4
		  AND ($5='' OR sr.version::text=$5) AND sr.is_active
		  AND (sr.expires_at IS NULL OR sr.expires_at>now()))`,
		tenantID, ref.Provider, ref.Scope, ref.Name, ref.Version).Scan(&exists)
	if err != nil {
		return fmt.Errorf("validate storage secret reference: %w", err)
	}
	if !exists {
		return errStorageInvalidRef
	}
	return nil
}

func (s *postgresStorageDesiredStore) ListBackends(ctx context.Context, tenantID string) ([]storageBackendRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+backendColumns+` FROM storage_backends WHERE tenant_id=$1 ORDER BY display_name,id`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list storage backends: %w", err)
	}
	defer rows.Close()
	items := []storageBackendRecord{}
	for rows.Next() {
		item, err := scanBackend(rows)
		if err != nil {
			return nil, fmt.Errorf("scan storage backend: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStorageDesiredStore) GetBackend(ctx context.Context, tenantID, id string) (storageBackendRecord, error) {
	item, err := scanBackend(s.db.QueryRowContext(ctx, `SELECT `+backendColumns+` FROM storage_backends WHERE tenant_id=$1 AND id=$2`, tenantID, id))
	return item, desiredScanError("get storage backend", err)
}

func secretArgs(ref *secretReference) (any, any, any, any) {
	if ref == nil {
		return nil, nil, nil, nil
	}
	var version any
	if ref.Version != "" {
		version = ref.Version
	}
	return ref.Provider, ref.Scope, ref.Name, version
}

func (s *postgresStorageDesiredStore) CreateBackend(ctx context.Context, tenantID, id string, input storageBackendInput) (storageBackendRecord, error) {
	p, scope, name, version := secretArgs(input.SecretReference)
	attributes := storageAttributesJSON(input.Attributes)
	item, err := scanBackend(s.db.QueryRowContext(ctx, `INSERT INTO storage_backends
		(id,tenant_id,provider_type,provider_schema_version,backend_id,display_name,description,secret_provider,secret_scope,secret_name,secret_version,attributes)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING `+backendColumns,
		id, tenantID, input.ProviderType, input.ProviderSchemaVersion, input.BackendID, input.DisplayName, input.Description, p, scope, name, version, attributes))
	return item, desiredWriteError("create storage backend", err)
}

func (s *postgresStorageDesiredStore) UpdateBackend(ctx context.Context, tenantID, id string, expected int64, input storageBackendInput) (storageBackendRecord, error) {
	p, scope, name, version := secretArgs(input.SecretReference)
	attributes := storageAttributesJSON(input.Attributes)
	item, err := scanBackend(s.db.QueryRowContext(ctx, `UPDATE storage_backends SET provider_type=$4,provider_schema_version=$5,backend_id=$6,display_name=$7,
		description=$8,secret_provider=$9,secret_scope=$10,secret_name=$11,secret_version=$12,attributes=$13,version=version+1,updated_at=now()
		WHERE tenant_id=$1 AND id=$2 AND version=$3 RETURNING `+backendColumns,
		tenantID, id, expected, input.ProviderType, input.ProviderSchemaVersion, input.BackendID, input.DisplayName, input.Description, p, scope, name, version, attributes))
	if err == sql.ErrNoRows {
		return item, s.conflictOrMissing(ctx, "storage_backends", tenantID, id)
	}
	return item, desiredWriteError("update storage backend", err)
}

func storageAttributesJSON(attributes map[string]any) []byte {
	if attributes == nil {
		return []byte(`{}`)
	}
	value, _ := json.Marshal(attributes)
	return value
}

func (s *postgresStorageDesiredStore) DeleteBackend(ctx context.Context, tenantID, id string, expected int64) error {
	return s.deleteVersioned(ctx, "storage_backends", tenantID, id, expected)
}

const offeringColumns = `id,tenant_id,COALESCE(backend_id::text,''),name,description,service_mode,access_modes,
	volume_expansion,snapshots,clones,topology,protection_class,version,created_at,updated_at`

func scanOffering(scanner interface{ Scan(...any) error }) (workloadStorageOfferingRecord, error) {
	var item workloadStorageOfferingRecord
	var access, topology []byte
	err := scanner.Scan(&item.ID, &item.TenantID, &item.BackendID, &item.Name, &item.Description, &item.ServiceMode, &access,
		&item.VolumeExpansion, &item.Snapshots, &item.Clones, &topology, &item.ProtectionClass, &item.Version, &item.CreatedAt, &item.UpdatedAt)
	if err == nil {
		err = json.Unmarshal(access, &item.AccessModes)
	}
	if err == nil {
		err = json.Unmarshal(topology, &item.Topology)
	}
	return item, err
}

func offeringJSON(input workloadStorageOfferingInput) (string, string) {
	access, _ := json.Marshal(input.AccessModes)
	topology, _ := json.Marshal(input.Topology)
	if input.Topology == nil {
		topology = []byte(`{}`)
	}
	return string(access), string(topology)
}

func nullableUUID(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func (s *postgresStorageDesiredStore) ListOfferings(ctx context.Context, tenantID string) ([]workloadStorageOfferingRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+offeringColumns+` FROM workload_storage_offerings WHERE tenant_id=$1 ORDER BY name,id`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list storage offerings: %w", err)
	}
	defer rows.Close()
	items := []workloadStorageOfferingRecord{}
	for rows.Next() {
		item, err := scanOffering(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStorageDesiredStore) GetOffering(ctx context.Context, tenantID, id string) (workloadStorageOfferingRecord, error) {
	item, err := scanOffering(s.db.QueryRowContext(ctx, `SELECT `+offeringColumns+` FROM workload_storage_offerings WHERE tenant_id=$1 AND id=$2`, tenantID, id))
	return item, desiredScanError("get storage offering", err)
}

func (s *postgresStorageDesiredStore) CreateOffering(ctx context.Context, tenantID, id string, input workloadStorageOfferingInput) (workloadStorageOfferingRecord, error) {
	access, topology := offeringJSON(input)
	item, err := scanOffering(s.db.QueryRowContext(ctx, `INSERT INTO workload_storage_offerings
		(id,tenant_id,backend_id,name,description,service_mode,access_modes,volume_expansion,snapshots,clones,topology,protection_class)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING `+offeringColumns,
		id, tenantID, nullableUUID(input.BackendID), input.Name, input.Description, input.ServiceMode, access, input.VolumeExpansion, input.Snapshots, input.Clones, topology, input.ProtectionClass))
	return item, desiredWriteError("create storage offering", err)
}

func (s *postgresStorageDesiredStore) UpdateOffering(ctx context.Context, tenantID, id string, expected int64, input workloadStorageOfferingInput) (workloadStorageOfferingRecord, error) {
	access, topology := offeringJSON(input)
	item, err := scanOffering(s.db.QueryRowContext(ctx, `UPDATE workload_storage_offerings SET backend_id=$4,name=$5,description=$6,
		service_mode=$7,access_modes=$8,volume_expansion=$9,snapshots=$10,clones=$11,topology=$12,protection_class=$13,
		version=version+1,updated_at=now() WHERE tenant_id=$1 AND id=$2 AND version=$3 RETURNING `+offeringColumns,
		tenantID, id, expected, nullableUUID(input.BackendID), input.Name, input.Description, input.ServiceMode, access, input.VolumeExpansion, input.Snapshots, input.Clones, topology, input.ProtectionClass))
	if err == sql.ErrNoRows {
		return item, s.conflictOrMissing(ctx, "workload_storage_offerings", tenantID, id)
	}
	return item, desiredWriteError("update storage offering", err)
}

func (s *postgresStorageDesiredStore) DeleteOffering(ctx context.Context, tenantID, id string, expected int64) error {
	return s.deleteVersioned(ctx, "workload_storage_offerings", tenantID, id, expected)
}

const bindingColumns = `id,tenant_id,offering_id,offering_version,target_id,storage_class_name,storage_class_uid,
	storage_class_resource_version,sync_state,is_default,source,observed_at,freshness,topology,conditions,version,created_at,updated_at`

func scanBinding(scanner interface{ Scan(...any) error }) (storageClassBindingRecord, error) {
	var item storageClassBindingRecord
	var topology, conditions []byte
	err := scanner.Scan(&item.ID, &item.TenantID, &item.OfferingID, &item.OfferingVersion, &item.TargetID, &item.StorageClassName,
		&item.StorageClassUID, &item.StorageClassResourceVersion, &item.SyncState, &item.IsDefault, &item.Source, &item.ObservedAt,
		&item.Freshness, &topology, &conditions, &item.Version, &item.CreatedAt, &item.UpdatedAt)
	if err == nil {
		err = json.Unmarshal(topology, &item.Topology)
	}
	if err == nil {
		err = json.Unmarshal(conditions, &item.Conditions)
	}
	return item, err
}

func bindingTopology(input storageClassBindingInput) string {
	value, _ := json.Marshal(input.Topology)
	if input.Topology == nil {
		return `{}`
	}
	return string(value)
}

func (s *postgresStorageDesiredStore) ListBindings(ctx context.Context, tenantID, offeringID string) ([]storageClassBindingRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+bindingColumns+` FROM storage_class_bindings WHERE tenant_id=$1 AND offering_id=$2 ORDER BY target_id,storage_class_name,id`, tenantID, offeringID)
	if err != nil {
		return nil, fmt.Errorf("list storage bindings: %w", err)
	}
	defer rows.Close()
	items := []storageClassBindingRecord{}
	for rows.Next() {
		item, err := scanBinding(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *postgresStorageDesiredStore) GetBinding(ctx context.Context, tenantID, id string) (storageClassBindingRecord, error) {
	item, err := scanBinding(s.db.QueryRowContext(ctx, `SELECT `+bindingColumns+` FROM storage_class_bindings WHERE tenant_id=$1 AND id=$2`, tenantID, id))
	return item, desiredScanError("get storage binding", err)
}

func (s *postgresStorageDesiredStore) CreateBinding(ctx context.Context, tenantID, offeringID, id string, input storageClassBindingInput) (storageClassBindingRecord, error) {
	item, err := scanBinding(s.db.QueryRowContext(ctx, `INSERT INTO storage_class_bindings
		(id,tenant_id,offering_id,offering_version,target_id,storage_class_name,storage_class_uid,storage_class_resource_version,sync_state,is_default,source,observed_at,freshness,topology)
		SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14
		FROM workload_storage_offerings WHERE id=$3 AND tenant_id=$2 AND version=$4 RETURNING `+bindingColumns,
		id, tenantID, offeringID, input.OfferingVersion, input.TargetID, input.StorageClassName, input.StorageClassUID, input.StorageClassResourceVersion, input.SyncState, input.IsDefault, input.Source, input.ObservedAt, bindingFreshness(input), bindingTopology(input)))
	if err == sql.ErrNoRows {
		return item, errStorageInvalidRef
	}
	return item, desiredWriteError("create storage binding", err)
}

func (s *postgresStorageDesiredStore) UpdateBinding(ctx context.Context, tenantID, id string, expected int64, input storageClassBindingInput) (storageClassBindingRecord, error) {
	item, err := scanBinding(s.db.QueryRowContext(ctx, `UPDATE storage_class_bindings SET offering_version=$4,target_id=$5,storage_class_name=$6,
		storage_class_uid=$7,storage_class_resource_version=$8,sync_state=$9,is_default=$10,source=$11,observed_at=$12,freshness=$13,topology=$14,
		version=version+1,updated_at=now() WHERE tenant_id=$1 AND id=$2 AND version=$3
		AND EXISTS (SELECT 1 FROM workload_storage_offerings o WHERE o.id=storage_class_bindings.offering_id AND o.tenant_id=$1 AND o.version=$4)
		RETURNING `+bindingColumns,
		tenantID, id, expected, input.OfferingVersion, input.TargetID, input.StorageClassName, input.StorageClassUID, input.StorageClassResourceVersion, input.SyncState, input.IsDefault, input.Source, input.ObservedAt, bindingFreshness(input), bindingTopology(input)))
	if err == sql.ErrNoRows {
		return item, s.conflictOrMissing(ctx, "storage_class_bindings", tenantID, id)
	}
	return item, desiredWriteError("update storage binding", err)
}

func bindingFreshness(input storageClassBindingInput) string { return input.Freshness }

func (s *postgresStorageDesiredStore) DeleteBinding(ctx context.Context, tenantID, id string, expected int64) error {
	return s.deleteVersioned(ctx, "storage_class_bindings", tenantID, id, expected)
}

func (s *postgresStorageDesiredStore) conflictOrMissing(ctx context.Context, table, tenantID, id string) error {
	var exists bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM `+table+` WHERE tenant_id=$1 AND id=$2)`, tenantID, id).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return errStorageVersionConflict
	}
	return errStorageDesiredNotFound
}

func (s *postgresStorageDesiredStore) deleteVersioned(ctx context.Context, table, tenantID, id string, expected int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM `+table+` WHERE tenant_id=$1 AND id=$2 AND version=$3`, tenantID, id, expected)
	if err != nil {
		return desiredWriteError("delete storage desired state", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 1 {
		return nil
	}
	return s.conflictOrMissing(ctx, table, tenantID, id)
}

func desiredScanError(operation string, err error) error {
	if err == sql.ErrNoRows {
		return errStorageDesiredNotFound
	}
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}
func desiredWriteError(operation string, err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pq.Error
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return errStorageAlreadyExists
		case "23503":
			return errStorageInvalidRef
		}
	}
	return fmt.Errorf("%s: %w", operation, err)
}
