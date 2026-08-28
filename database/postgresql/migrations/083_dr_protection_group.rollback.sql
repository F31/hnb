-- Migration: 083_dr_protection_group (rollback)
-- Description: Drop the DR protection group orchestration tables.

BEGIN;

DROP TABLE IF EXISTS dr_switch_runs;
DROP TABLE IF EXISTS dr_group_members;
DROP TABLE IF EXISTS dr_protection_groups;

COMMIT;
