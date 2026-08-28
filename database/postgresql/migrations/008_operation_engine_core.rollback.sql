-- Rollback: 008_operation_engine_core
DROP TABLE IF EXISTS operation_read_model CASCADE;
DROP TABLE IF EXISTS operation_audit CASCADE;
DROP TABLE IF EXISTS compensation_records CASCADE;
DROP TABLE IF EXISTS step_checkpoints CASCADE;
DROP TABLE IF EXISTS operation_steps CASCADE;
DROP TABLE IF EXISTS operations CASCADE;
DROP TABLE IF EXISTS execution_plans CASCADE;
