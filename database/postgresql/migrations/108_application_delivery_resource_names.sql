BEGIN;
ALTER TABLE application_delivery_bindings
    ADD COLUMN IF NOT EXISTS workload_name TEXT,
    ADD COLUMN IF NOT EXISTS service_name TEXT;
COMMIT;
