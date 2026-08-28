-- Rollback: 012_gateway_engine
DROP TABLE IF EXISTS gateway_capability_snapshots CASCADE;
DROP TABLE IF EXISTS reference_grants CASCADE;
DROP TABLE IF EXISTS http_routes CASCADE;
DROP TABLE IF EXISTS gateway_profiles CASCADE;
DROP TABLE IF EXISTS gateways CASCADE;
DROP TABLE IF EXISTS gateway_classes CASCADE;
