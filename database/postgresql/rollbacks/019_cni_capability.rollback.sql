-- Rollback: 019_cni_capability
-- Description: Remove structured CNI capability columns from capability_snapshots

ALTER TABLE capability_snapshots DROP COLUMN IF EXISTS cni_details;