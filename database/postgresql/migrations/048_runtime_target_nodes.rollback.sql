-- Migration: 048_runtime_target_nodes (rollback)
DROP TABLE IF EXISTS bff_runtime_intents;
DROP TABLE IF EXISTS runtime_target_nodes;
