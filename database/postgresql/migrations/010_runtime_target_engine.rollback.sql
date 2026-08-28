-- Rollback: 010_runtime_target_engine
DROP TABLE IF EXISTS provider_registry CASCADE;
DROP TABLE IF EXISTS capability_snapshots CASCADE;
DROP TABLE IF EXISTS runtime_targets CASCADE;
