package nats

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	natslib "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const relayBatchSize = 100

type OutboxRelay struct {
	db           *sql.DB
	js           jetstream.JetStream
	pollInterval time.Duration
}

func NewOutboxRelay(db *sql.DB, nc *natslib.Conn, pollInterval time.Duration) (*OutboxRelay, error) {
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("jetstream: %w", err)
	}
	return &OutboxRelay{db: db, js: js, pollInterval: pollInterval}, nil
}

func (r *OutboxRelay) Start(ctx context.Context) error {
	if err := r.ensureStream(ctx); err != nil {
		return err
	}

	ticker := time.NewTicker(r.pollInterval)
	defer ticker.Stop()
	for {
		if err := r.publishBatch(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("outbox relay: %v", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (r *OutboxRelay) ensureStream(ctx context.Context) error {
	_, err := r.js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:       domainEventStream,
		Subjects:   []string{"hnb.event.>"},
		Storage:    jetstream.FileStorage,
		Retention:  jetstream.LimitsPolicy,
		MaxAge:     30 * 24 * time.Hour,
		MaxBytes:   5 << 30,
		MaxMsgSize: 2 << 20,
		Discard:    jetstream.DiscardOld,
		Duplicates: 2 * time.Minute,
	})
	if err != nil {
		return fmt.Errorf("domain-event stream: %w", err)
	}
	return nil
}

func (r *OutboxRelay) publishBatch(ctx context.Context) error {
	for i := 0; i < relayBatchSize; i++ {
		published, err := r.publishOne(ctx)
		if err != nil {
			return err
		}
		if !published {
			return nil
		}
	}
	return nil
}

type outboxRecord struct {
	id               string
	messageID        string
	messageType      string
	schemaVersion    string
	subject          string
	occurredAt       time.Time
	tenantID         string
	projectID        sql.NullString
	environmentID    sql.NullString
	actorID          sql.NullString
	correlationID    string
	causationID      sql.NullString
	idempotencyKey   string
	aggregateID      sql.NullString
	aggregateVersion sql.NullInt64
	operationID      sql.NullString
	stepID           sql.NullString
	expectedVersion  sql.NullInt64
	payload          []byte
	attempt          int
	maxAttempts      int
}

func (r *OutboxRelay) publishOne(ctx context.Context) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin relay transaction: %w", err)
	}
	defer tx.Rollback()

	var record outboxRecord
	err = tx.QueryRowContext(ctx, `
		SELECT id, message_id, message_type, schema_version, subject, occurred_at,
			tenant_id, project_id, environment_id, actor_id, correlation_id,
			causation_id, idempotency_key, aggregate_id, aggregate_version,
			operation_id, step_id, expected_version, payload, attempt, max_attempts
		FROM outbox_events
		WHERE status = 'pending' AND next_attempt_at <= now() AND attempt < max_attempts
		ORDER BY next_attempt_at, created_at
		LIMIT 1
		FOR UPDATE SKIP LOCKED`,
	).Scan(
		&record.id, &record.messageID, &record.messageType, &record.schemaVersion,
		&record.subject, &record.occurredAt, &record.tenantID, &record.projectID,
		&record.environmentID, &record.actorID, &record.correlationID,
		&record.causationID, &record.idempotencyKey, &record.aggregateID,
		&record.aggregateVersion, &record.operationID, &record.stepID,
		&record.expectedVersion, &record.payload, &record.attempt, &record.maxAttempts,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("select pending outbox event: %w", err)
	}

	envelope := eventEnvelope{
		MessageID:      record.messageID,
		MessageType:    record.messageType,
		SchemaVersion:  record.schemaVersion,
		OccurredAt:     record.occurredAt,
		TenantID:       record.tenantID,
		ProjectID:      record.projectID.String,
		EnvironmentID:  record.environmentID.String,
		ActorID:        record.actorID.String,
		CorrelationID:  record.correlationID,
		CausationID:    record.causationID.String,
		IdempotencyKey: record.idempotencyKey,
		AggregateID:    record.aggregateID.String,
		OperationID:    record.operationID.String,
		StepID:         record.stepID.String,
		Payload:        json.RawMessage(record.payload),
	}
	if record.aggregateVersion.Valid {
		envelope.AggregateVersion = record.aggregateVersion.Int64
	}
	if record.expectedVersion.Valid {
		envelope.ExpectedVersion = record.expectedVersion.Int64
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		return false, r.markFailedAttempt(ctx, tx, record, fmt.Errorf("marshal envelope: %w", err))
	}
	message := natslib.NewMsg(record.subject)
	message.Header.Set(jetstream.MsgIDHeader, record.messageID)
	message.Data = payload
	if _, err := r.js.PublishMsg(ctx, message); err != nil {
		return false, r.markFailedAttempt(ctx, tx, record, err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE outbox_events SET
			status = 'published', published_at = now(), updated_at = now(), last_error = NULL
		WHERE id = $1`, record.id,
	); err != nil {
		return false, fmt.Errorf("mark outbox event published: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit published outbox event: %w", err)
	}
	return true, nil
}

func (r *OutboxRelay) markFailedAttempt(
	ctx context.Context,
	tx *sql.Tx,
	record outboxRecord,
	publishErr error,
) error {
	delay := retryDelay(record.attempt + 1)
	_, err := tx.ExecContext(ctx, `
		UPDATE outbox_events SET
			attempt = attempt + 1,
			status = CASE WHEN attempt + 1 >= max_attempts THEN 'failed' ELSE 'pending' END,
			next_attempt_at = now() + ($2 * interval '1 second'),
			last_error = $3,
			updated_at = now()
		WHERE id = $1`, record.id, int(delay.Seconds()), publishErr.Error(),
	)
	if err != nil {
		return fmt.Errorf("record outbox publish failure: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit outbox publish failure: %w", err)
	}
	return fmt.Errorf("publish outbox event %s: %w", record.messageID, publishErr)
}

func retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := 5 * time.Second * time.Duration(1<<min(attempt-1, 6))
	if delay > 5*time.Minute {
		return 5 * time.Minute
	}
	return delay
}
