package observer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	observationsAccepted = promauto.NewCounter(prometheus.CounterOpts{
		Name: "hnb_observation_projected_total",
		Help: "Observations committed by the runtime-target projector",
	})
	observationsReplay = promauto.NewCounter(prometheus.CounterOpts{
		Name: "hnb_observation_replay_total",
		Help: "Duplicate observations resolved idempotently",
	})
	observationsGapped = promauto.NewCounter(prometheus.CounterOpts{
		Name: "hnb_observation_sequence_gap_total",
		Help: "Observations rejected due to a sequence gap awaiting replay",
	})
	observationsFenced = promauto.NewCounter(prometheus.CounterOpts{
		Name: "hnb_observation_fenced_total",
		Help: "Observations fenced by a newer observer generation",
	})
	observationsRejected = promauto.NewCounter(prometheus.CounterOpts{
		Name: "hnb_observation_rejected_total",
		Help: "Observations rejected during identity/payload validation",
	})
	observationsOutOfOrder = promauto.NewCounter(prometheus.CounterOpts{
		Name: "hnb_observation_out_of_order_total",
		Help: "Observations that conflicted with a committed sequence",
	})
	observerGenerationJumps = promauto.NewCounter(prometheus.CounterOpts{
		Name: "hnb_observer_generation_jump_total",
		Help: "Attempted observer generation jumps without a source reset",
	})
	projectionLagSeconds = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "hnb_projection_lag_seconds",
		Help: "Lag between the oldest unprocessed observation and now",
	})
	sourceResetsAccepted = promauto.NewCounter(prometheus.CounterOpts{
		Name: "hnb_source_reset_accepted_total",
		Help: "Observer source resets committed",
	})
)

// Store is the projector's data-access boundary. Only the projector touches
// these tables; Read Models never call observers directly.
type Store interface {
	// LoadCursor returns the current cursor for (tenant, target, observer), or
	// (false, nil) when no observation has been accepted yet.
	LoadCursor(ctx context.Context, tenantID, targetID, observerID string) (cursor, bool, error)
	// SaveObservation atomically applies the observation in a single
	// transaction: target projection, capability snapshot, nodes, cursor, and
	// inbox state.
	SaveObservation(ctx context.Context, o *Observation, identity Identity, digest string) error
	// ApplySourceReset atomically establishes a new observer generation.
	ApplySourceReset(ctx context.Context, reset *SourceReset, identity Identity) error
	// RecordReplay marks a duplicate observation as processed in the inbox.
	RecordReplay(ctx context.Context, o *Observation) error
	// RecordGap records a sequence gap request and returns false when the
	// observation cannot be applied yet.
	RecordGap(ctx context.Context, o *Observation, expectedNext int64) error
}

// Projector accepts tenant-bound observations, enforces ordering invariants,
// and projects them atomically into the Read Model.
type Projector struct {
	store Store
}

func NewProjector(store Store) *Projector {
	return &Projector{store: store}
}

// Accept validates, orders, and projects one observation. It returns ErrReplay
// for idempotent duplicates (not an error), ErrGap when a sequence gap is
// detected, and ErrFenced for stale generations.
func (p *Projector) Accept(ctx context.Context, identity Identity, payload []byte) error {
	o, err := ValidateObservation(identity, payload)
	if err != nil {
		observationsRejected.Inc()
		return fmt.Errorf("validate observation: %w", err)
	}
	current, exists, err := p.store.LoadCursor(ctx, o.TenantID, o.TargetID, o.ObserverID)
	if err != nil {
		return fmt.Errorf("load cursor: %w", err)
	}
	if !exists {
		if o.ObserverGeneration != 1 || o.Sequence != 1 {
			_ = p.store.RecordGap(ctx, o, 1)
			observationsGapped.Inc()
			return ErrGap
		}
		observationsAccepted.Inc()
		return p.store.SaveObservation(ctx, o, identity, o.Digest())
	}
	if o.ObserverGeneration < current.ObserverGeneration {
		observationsFenced.Inc()
		return ErrFenced
	}
	if o.ObserverGeneration > current.ObserverGeneration {
		// A new generation may only be established via source-reset, never by
		// an observation jump.
		observerGenerationJumps.Inc()
		observationsFenced.Inc()
		return ErrFenced
	}
	if o.Sequence < current.Sequence {
		observationsReplay.Inc()
		return ErrReplay
	}
	if o.Sequence == current.Sequence {
		if o.EventID == current.LastMessageID || o.Digest() == current.PayloadDigest {
			observationsReplay.Inc()
			return ErrReplay
		}
		observationsOutOfOrder.Inc()
		return fmt.Errorf("observation sequence %d conflicts with committed sequence", o.Sequence)
	}
	if o.Sequence > current.Sequence+1 {
		_ = p.store.RecordGap(ctx, o, current.Sequence+1)
		observationsGapped.Inc()
		return ErrGap
	}
	observationsAccepted.Inc()
	return p.store.SaveObservation(ctx, o, identity, o.Digest())
}

// ApplyReset processes a source-reset control message that establishes a new
// observer generation and re-authenticated lease.
func (p *Projector) ApplyReset(ctx context.Context, identity Identity, payload []byte) error {
	reset, err := ValidateSourceReset(identity, payload)
	if err != nil {
		observationsRejected.Inc()
		return fmt.Errorf("validate source-reset: %w", err)
	}
	if err := p.store.ApplySourceReset(ctx, reset, identity); err != nil {
		return err
	}
	sourceResetsAccepted.Inc()
	return nil
}

// ReportLag records the current projector lag for the oldest unprocessed
// inbox message. Callers poll this on an interval.
func (p *Projector) ReportLag(ctx context.Context, now time.Time) error {
	if p.store == nil {
		return nil
	}
	lagStore, ok := p.store.(interface {
		OldestUnprocessedObservedAt(context.Context) (time.Time, bool, error)
	})
	if !ok {
		return nil
	}
	oldest, ok, err := lagStore.OldestUnprocessedObservedAt(ctx)
	if err != nil || !ok {
		return err
	}
	projectionLagSeconds.Set(now.Sub(oldest).Seconds())
	return nil
}

// StoreCursor represents the database cursor row for the observation inbox.
type CursorState struct {
	TenantID           string
	TargetID           string
	ObserverID         string
	ObserverGeneration int64
	Sequence           int64
	PayloadDigest      string
	LastMessageID      string
	ObservedAt         time.Time
}

// PGCursorStore implements Store on PostgreSQL using the migration-051 tables.
type PGCursorStore struct {
	db *sql.DB
}

func NewPGCursorStore(db *sql.DB) *PGCursorStore {
	return &PGCursorStore{db: db}
}

func (s *PGCursorStore) LoadCursor(ctx context.Context, tenantID, targetID, observerID string) (cursor, bool, error) {
	var row cursor
	err := s.db.QueryRowContext(ctx, `
		SELECT tenant_id, target_id, observation_source_id, observation_source,
		       observation_generation, observation_revision, payload_digest,
		       last_message_id, observed_at
		FROM runtime_target_observation_cursors
		WHERE tenant_id = $1 AND target_id = $2 AND observation_source_id = $3`,
		tenantID, targetID, observerID,
	).Scan(&row.TenantID, &row.TargetID, &row.ObserverID, &row.ObserverKind,
		&row.ObserverGeneration, &row.Sequence, &row.PayloadDigest,
		&row.LastMessageID, &row.ObservedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return cursor{}, false, nil
	}
	if err != nil {
		return cursor{}, false, err
	}
	return row, true, nil
}

func (s *PGCursorStore) SaveObservation(ctx context.Context, o *Observation, identity Identity, digest string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if o.Target != nil {
		if _, err := tx.ExecContext(ctx, `
			UPDATE runtime_targets
			SET lifecycle_state = $1, health_state = $2, connectivity_state = $3,
			    last_known_state_at = $4, stale_threshold_seconds = $5,
			    observation_source = $6, observation_source_id = $7,
			    observation_generation = $8, observation_revision = $9,
			    observed_at = $10, updated_at = now()
			WHERE id = $11 AND tenant_id = $12`,
			o.Target.LifecycleState, o.Target.HealthState, o.Target.ConnectivityState,
			o.Target.LastKnownStateAt, o.Target.StaleThresholdSeconds,
			observerSourceFor(o.TargetKind), identity.ObserverID,
			o.ObserverGeneration, o.Sequence, o.ObservedAt,
			o.TargetID, o.TenantID); err != nil {
			return fmt.Errorf("project target: %w", err)
		}
	}

	if o.Capability != nil {
		if err := insertCapabilitySnapshot(ctx, tx, o, identity, digest); err != nil {
			return err
		}
	}

	if o.Nodes != nil {
		if err := projectNodes(ctx, tx, o, identity); err != nil {
			return err
		}
	}

	if o.StorageInventory != nil {
		if _, err := tx.ExecContext(ctx, `
			UPDATE runtime_targets SET projection_version = projection_version + 1, updated_at = now()
			WHERE id = $1 AND tenant_id = $2`, o.TargetID, o.TenantID); err != nil {
			return fmt.Errorf("advance storage projection version: %w", err)
		}
		if err := projectStorageInventory(ctx, tx, o, identity); err != nil {
			return err
		}
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO runtime_target_observation_cursors (
			tenant_id, target_id, observation_source, observation_source_id,
			observation_generation, observation_revision, payload_digest,
			last_message_id, observed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (tenant_id, target_id, observation_source, observation_source_id)
		DO UPDATE SET
			observation_generation = EXCLUDED.observation_generation,
			observation_revision = EXCLUDED.observation_revision,
			payload_digest = EXCLUDED.payload_digest,
			last_message_id = EXCLUDED.last_message_id,
			observed_at = EXCLUDED.observed_at,
			updated_at = now()`,
		o.TenantID, o.TargetID, observerSourceFor(o.TargetKind), identity.ObserverID,
		o.ObserverGeneration, o.Sequence, digest, o.EventID, o.ObservedAt); err != nil {
		return fmt.Errorf("update cursor: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO runtime_target_observation_inbox (
			message_id, tenant_id, target_id, observation_source,
			observation_source_id, observation_generation, observation_revision,
			payload_digest, observed_at, processed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())
		ON CONFLICT (message_id) DO UPDATE SET
			payload_digest = EXCLUDED.payload_digest,
			observed_at = EXCLUDED.observed_at,
			processed_at = now(),
			processing_error = NULL`,
		o.EventID, o.TenantID, o.TargetID, observerSourceFor(o.TargetKind),
		identity.ObserverID, o.ObserverGeneration, o.Sequence, digest, o.ObservedAt); err != nil {
		return fmt.Errorf("record inbox: %w", err)
	}
	return tx.Commit()
}

func (s *PGCursorStore) ApplySourceReset(ctx context.Context, reset *SourceReset, identity Identity) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var previous int64
	err = tx.QueryRowContext(ctx, `
		SELECT observation_generation FROM runtime_target_observation_cursors
		WHERE tenant_id = $1 AND target_id = $2 AND observation_source_id = $3
		FOR UPDATE`, reset.TenantID, reset.TargetID, reset.ObserverID).Scan(&previous)
	if errors.Is(err, sql.ErrNoRows) {
		// No established generation; a source-reset cannot establish one from
		// nothing. Callers must first accept an initial observation.
		return fmt.Errorf("source-reset on unknown observer generation")
	}
	if err != nil {
		return err
	}
	if previous != reset.PreviousObserverGeneration {
		return fmt.Errorf("source-reset previousObserverGeneration %d does not match committed %d", reset.PreviousObserverGeneration, previous)
	}
	if reset.NewObserverGeneration <= previous {
		return fmt.Errorf("source-reset newObserverGeneration must exceed committed generation %d", previous)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE runtime_target_observation_cursors
		SET observation_generation = $1, observation_revision = 0,
		    payload_digest = '', observed_at = $2, updated_at = now()
		WHERE tenant_id = $3 AND target_id = $4 AND observation_source_id = $5`,
		reset.NewObserverGeneration, reset.ObservedAt, reset.TenantID, reset.TargetID, reset.ObserverID); err != nil {
		return fmt.Errorf("apply source-reset cursor: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO runtime_target_observation_inbox (
			message_id, tenant_id, target_id, observation_source,
			observation_source_id, observation_generation, observation_revision,
			payload_digest, observed_at, processed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, '', $8, now())`,
		reset.EventID, reset.TenantID, reset.TargetID, observerSourceFor(reset.TargetKind),
		reset.ObserverID, reset.NewObserverGeneration, int64(0), reset.ObservedAt); err != nil {
		return fmt.Errorf("record source-reset inbox: %w", err)
	}
	return tx.Commit()
}

func (s *PGCursorStore) RecordReplay(ctx context.Context, o *Observation) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO runtime_target_observation_inbox (
			message_id, tenant_id, target_id, observation_source,
			observation_source_id, observation_generation, observation_revision,
			payload_digest, observed_at, processed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())
		ON CONFLICT (message_id) DO NOTHING`,
		o.EventID, o.TenantID, o.TargetID, observerSourceFor(o.TargetKind),
		o.ObserverID, o.ObserverGeneration, o.Sequence, o.Digest(), o.ObservedAt)
	return err
}

func (s *PGCursorStore) RecordGap(ctx context.Context, o *Observation, expectedNext int64) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO runtime_target_observation_inbox (
			message_id, tenant_id, target_id, observation_source,
			observation_source_id, observation_generation, observation_revision,
			payload_digest, observed_at, processed_at, processing_error
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NULL, $10)
		ON CONFLICT (message_id) DO UPDATE SET processing_error = EXCLUDED.processing_error`,
		o.EventID, o.TenantID, o.TargetID, observerSourceFor(o.TargetKind),
		o.ObserverID, o.ObserverGeneration, o.Sequence, o.Digest(), o.ObservedAt,
		fmt.Sprintf("sequence gap: expected %d", expectedNext))
	return err
}

// OldestUnprocessedObservedAt returns the observedAt of the oldest inbox
// message that has not been processed, used to derive projection lag.
func (s *PGCursorStore) OldestUnprocessedObservedAt(ctx context.Context) (time.Time, bool, error) {
	var oldest time.Time
	err := s.db.QueryRowContext(ctx, `
		SELECT observed_at FROM runtime_target_observation_inbox
		WHERE processed_at IS NULL
		ORDER BY observed_at ASC LIMIT 1`).Scan(&oldest)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return oldest, true, nil
}

func insertCapabilitySnapshot(ctx context.Context, tx *sql.Tx, o *Observation, identity Identity, digest string) error {
	content, err := json.Marshal(o.Capability.Content)
	if err != nil {
		return fmt.Errorf("marshal capability content: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO capability_snapshots (
			target_id, tenant_id, target_kind, observation_source,
			observation_source_id, observation_generation, observation_revision,
			content_digest, snapshot_json, observed_at, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, $10, now())
		ON CONFLICT (tenant_id, target_id, content_digest) WHERE content_digest IS NOT NULL DO NOTHING`,
		o.TargetID, o.TenantID, o.TargetKind, observerSourceFor(o.TargetKind),
		identity.ObserverID, o.ObserverGeneration, o.Sequence, digest, string(content), o.ObservedAt)
	if err != nil {
		return fmt.Errorf("insert capability snapshot: %w", err)
	}
	return nil
}

func projectNodes(ctx context.Context, tx *sql.Tx, o *Observation, identity Identity) error {
	existing, err := currentNodeIDs(ctx, tx, o.TenantID, o.TargetID)
	if err != nil {
		return err
	}
	present := make(map[string]bool, len(o.Nodes))
	for _, node := range o.Nodes {
		present[node.NodeID] = true
		if node.Deleted {
			// Delta tombstone: mark absent, never physical delete.
			if _, err := tx.ExecContext(ctx, `
				UPDATE runtime_target_nodes
				SET deleted_at = $1, updated_at = now()
				WHERE tenant_id = $2 AND target_id = $3 AND source_node_uid = $4`,
				o.ObservedAt, o.TenantID, o.TargetID, node.NodeID); err != nil {
				return fmt.Errorf("tombstone node: %w", err)
			}
			continue
		}
		resources, err := json.Marshal(node.Resources)
		if err != nil {
			return fmt.Errorf("marshal node resources: %w", err)
		}
		labels, err := json.Marshal(node.Labels)
		if err != nil {
			return fmt.Errorf("marshal node labels: %w", err)
		}
		role := "worker"
		if o.TargetKind == "KubernetesTarget" && node.Labels != nil && node.Labels["node-role.kubernetes.io/control-plane"] != "" {
			role = "control_plane"
		}
		if o.TargetKind == "EdgeRuntimeTarget" {
			role = "edge"
		}
		status := nodeStatus(node.ConnectivityState, node.HealthState)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO runtime_target_nodes (
				target_id, tenant_id, source_node_uid, name, role, node_status,
				arch, kubelet_version, observed_at, last_known_state_at,
				lifecycle_state, health_state, connectivity_state,
				observation_source, observation_source_id,
				observation_generation, observation_revision,
				labels, capacity, deleted_at, created_at, updated_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18::jsonb,$19::jsonb,NULL,now(),now())
			ON CONFLICT (tenant_id, target_id, source_node_uid) WHERE source_node_uid IS NOT NULL
			DO UPDATE SET
				name = EXCLUDED.name, role = EXCLUDED.role, node_status = EXCLUDED.node_status,
				arch = EXCLUDED.arch, kubelet_version = EXCLUDED.kubelet_version,
				observed_at = EXCLUDED.observed_at, last_known_state_at = EXCLUDED.last_known_state_at,
				lifecycle_state = EXCLUDED.lifecycle_state, health_state = EXCLUDED.health_state,
				connectivity_state = EXCLUDED.connectivity_state,
				observation_source = EXCLUDED.observation_source,
				observation_source_id = EXCLUDED.observation_source_id,
				observation_generation = EXCLUDED.observation_generation,
				observation_revision = EXCLUDED.observation_revision,
				labels = EXCLUDED.labels, capacity = EXCLUDED.capacity,
				deleted_at = NULL, updated_at = now()`,
			o.TargetID, o.TenantID, node.NodeID, node.Name, role, status,
			node.Architecture, node.KubeletVersion, node.ObservedAt, node.LastKnownStateAt,
			node.LifecycleState, node.HealthState, node.ConnectivityState,
			observerSourceFor(o.TargetKind), identity.ObserverID,
			o.ObserverGeneration, o.Sequence, string(labels), string(resources)); err != nil {
			return fmt.Errorf("upsert node: %w", err)
		}
	}
	if o.InventoryMode == "Full" {
		// Full inventory: any previously-observed node absent from this report
		// becomes a timestamped tombstone, never a physical delete.
		for id := range existing {
			if !present[id] {
				if _, err := tx.ExecContext(ctx, `
					UPDATE runtime_target_nodes
					SET deleted_at = $1, updated_at = now()
					WHERE tenant_id = $2 AND target_id = $3 AND source_node_uid = $4`,
					o.ObservedAt, o.TenantID, o.TargetID, id); err != nil {
					return fmt.Errorf("tombstone missing node: %w", err)
				}
			}
		}
	}
	return nil
}

func currentNodeIDs(ctx context.Context, tx *sql.Tx, tenantID, targetID string) (map[string]bool, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT source_node_uid FROM runtime_target_nodes
		WHERE tenant_id = $1 AND target_id = $2 AND deleted_at IS NULL AND source_node_uid IS NOT NULL`,
		tenantID, targetID)
	if err != nil {
		return nil, fmt.Errorf("list current nodes: %w", err)
	}
	defer rows.Close()
	ids := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids[id] = true
	}
	return ids, rows.Err()
}

func observerSourceFor(targetKind string) string {
	if targetKind == "EdgeRuntimeTarget" {
		return "cloudcore"
	}
	return "agent"
}

func nodeStatus(connectivity, health string) string {
	if connectivity == "CONNECTED" {
		return "Ready"
	}
	if connectivity == "DISCONNECTED" {
		return "NotReady"
	}
	return "Unknown"
}

var _ = uuid.NewString
