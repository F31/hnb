-- Rollback: 001_nats_jetstream_outbox
-- Description: Revert NATS JetStream Outbox extensions

DROP TABLE IF EXISTS consumer_checkpoints;
DROP TABLE IF EXISTS worker_leases;
DROP TABLE IF EXISTS outbox_events;
