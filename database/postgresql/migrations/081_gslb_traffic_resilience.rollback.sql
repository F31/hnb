-- Migration: 081_gslb_traffic_resilience (rollback)
-- Description: Drop the GSLB traffic resilience tables.

BEGIN;

DROP TABLE IF EXISTS gslb_switch_requests;
DROP TABLE IF EXISTS gslb_read_model;
DROP TABLE IF EXISTS gslb_health_checks;
DROP TABLE IF EXISTS gslb_pool_members;
DROP TABLE IF EXISTS gslb_pools;
DROP TABLE IF EXISTS gslb_services;

COMMIT;
