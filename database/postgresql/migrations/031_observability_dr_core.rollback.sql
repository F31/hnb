-- 031_observability_dr_core.rollback.sql
BEGIN;

DROP TABLE IF EXISTS operation_slo_alerts;
DROP TABLE IF EXISTS operation_slo_config;

COMMIT;