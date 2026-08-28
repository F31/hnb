-- Migration: 001_nats_jetstream_outbox
-- Description: Add NATS JetStream Outbox, Worker Lease, Consumer Checkpoint, and Idempotency tables
-- Tiers: All
-- Dependencies: pgcrypto extension for gen_random_uuid()

-- 1. Transactional outbox. This migration is the bootstrap owner of the table.
CREATE TABLE IF NOT EXISTS outbox_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id UUID NOT NULL DEFAULT gen_random_uuid(),
    message_type TEXT NOT NULL,
    schema_version TEXT NOT NULL,
    subject TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    tenant_id TEXT NOT NULL,
    project_id TEXT,
    environment_id TEXT,
    actor_id TEXT,
    correlation_id UUID NOT NULL,
    causation_id UUID,
    idempotency_key TEXT NOT NULL,
    aggregate_id TEXT,
    aggregate_version BIGINT,
    operation_id UUID,
    step_id UUID,
    resource_id TEXT,
    expected_version BIGINT,
    payload JSONB NOT NULL,
    payload_ref TEXT,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'published', 'failed')),
    attempt INTEGER NOT NULL DEFAULT 0 CHECK (attempt >= 0),
    max_attempts INTEGER NOT NULL DEFAULT 10 CHECK (max_attempts > 0),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_error TEXT,
    published_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (message_id),
    UNIQUE (message_type, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_outbox_events_operation_id ON outbox_events(operation_id);
CREATE INDEX IF NOT EXISTS idx_outbox_events_step_id ON outbox_events(step_id);
CREATE INDEX IF NOT EXISTS idx_outbox_events_pending
  ON outbox_events(next_attempt_at, created_at)
  WHERE status = 'pending';

-- 2. WorkerLease table for fencing and concurrency control
CREATE TABLE IF NOT EXISTS worker_leases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    step_id UUID NOT NULL,
    owner_id TEXT NOT NULL,
    fencing_token UUID NOT NULL DEFAULT gen_random_uuid(),
    acquired_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_worker_leases_step_id ON worker_leases(step_id);
CREATE INDEX IF NOT EXISTS idx_worker_leases_expires_at ON worker_leases(expires_at);
CREATE INDEX IF NOT EXISTS idx_worker_leases_owner_id ON worker_leases(owner_id);

-- 3. ConsumerCheckpoint table for replay tracking
CREATE TABLE IF NOT EXISTS consumer_checkpoints (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    consumer_name TEXT NOT NULL,
    stream_name TEXT NOT NULL,
    last_sequence BIGINT NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_consumer_checkpoints_name ON consumer_checkpoints(consumer_name, stream_name);
