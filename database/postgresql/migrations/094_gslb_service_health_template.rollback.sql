-- Rollback: 094_gslb_service_health_template
DROP INDEX IF EXISTS gslb_services_tenant_domain_key;
ALTER TABLE gslb_services
    DROP COLUMN IF EXISTS health_failure_threshold,
    DROP COLUMN IF EXISTS health_timeout_seconds,
    DROP COLUMN IF EXISTS health_interval_seconds,
    DROP COLUMN IF EXISTS health_path,
    DROP COLUMN IF EXISTS health_protocol;
