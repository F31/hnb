-- Rollback: 013_operation_fencing_v2
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM worker_leases) THEN
        RAISE EXCEPTION 'rollback 013 requires worker_leases to be empty';
    END IF;
END
$$;

ALTER TABLE worker_leases
    DROP CONSTRAINT fk_worker_leases_step_id,
    DROP COLUMN fencing_generation;
ALTER TABLE worker_leases
    RENAME COLUMN lease_id TO fencing_token;

ALTER TABLE operation_steps
    DROP COLUMN fencing_generation;
ALTER TABLE operation_steps
    RENAME COLUMN last_lease_id TO fencing_token;

ALTER INDEX idx_operation_steps_last_lease_id
    RENAME TO idx_operation_steps_fencing_token;

DROP SEQUENCE operation_fencing_generation_seq;
