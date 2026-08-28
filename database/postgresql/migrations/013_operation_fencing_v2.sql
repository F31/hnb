-- Migration: 013_operation_fencing_v2
-- Description: Add monotonic operation Step fencing generations
-- Tiers: All
-- Dependencies: 001_nats_jetstream_outbox (worker_leases), 008_operation_engine_core (operation_steps)

CREATE SEQUENCE IF NOT EXISTS operation_fencing_generation_seq
    AS BIGINT
    MINVALUE 1
    START WITH 1
    INCREMENT BY 1
    NO CYCLE;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM worker_leases) AND EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public' AND table_name = 'worker_leases' AND column_name = 'fencing_token'
    ) THEN
        RAISE EXCEPTION 'migration 013 requires worker_leases to be empty before the first upgrade';
    END IF;

    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public' AND table_name = 'worker_leases' AND column_name = 'fencing_token'
    ) THEN
        ALTER TABLE worker_leases RENAME COLUMN fencing_token TO lease_id;
    END IF;

    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public' AND table_name = 'operation_steps' AND column_name = 'fencing_token'
    ) THEN
        ALTER TABLE operation_steps RENAME COLUMN fencing_token TO last_lease_id;
    END IF;
END
$$;

ALTER TABLE worker_leases
    ADD COLUMN IF NOT EXISTS fencing_generation BIGINT NOT NULL DEFAULT nextval('operation_fencing_generation_seq')
        CHECK (fencing_generation > 0);

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_worker_leases_step_id') THEN
        ALTER TABLE worker_leases ADD CONSTRAINT fk_worker_leases_step_id
            FOREIGN KEY (step_id) REFERENCES operation_steps(id) ON DELETE CASCADE;
    END IF;
END
$$;

ALTER TABLE operation_steps
    ADD COLUMN IF NOT EXISTS fencing_generation BIGINT NOT NULL DEFAULT 0 CHECK (fencing_generation >= 0);

DO $$
BEGIN
    IF to_regclass('idx_operation_steps_fencing_token') IS NOT NULL
       AND to_regclass('idx_operation_steps_last_lease_id') IS NULL THEN
        ALTER INDEX idx_operation_steps_fencing_token RENAME TO idx_operation_steps_last_lease_id;
    END IF;
END
$$;
