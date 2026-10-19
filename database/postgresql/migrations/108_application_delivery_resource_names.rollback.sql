BEGIN;
ALTER TABLE application_delivery_bindings DROP COLUMN IF EXISTS service_name;
ALTER TABLE application_delivery_bindings DROP COLUMN IF EXISTS workload_name;
COMMIT;
