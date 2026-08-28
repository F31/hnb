-- Migration: 082_gslb_operation_wiring (rollback)
-- Description: Remove GSLB operation wiring, drill reports and DR group reference.

BEGIN;

ALTER TABLE gslb_read_model
    DROP COLUMN IF EXISTS last_drill_report_id,
    DROP COLUMN IF EXISTS last_drill_verdict,
    DROP COLUMN IF EXISTS last_drill_at;

DROP TABLE IF EXISTS gslb_drill_reports;

ALTER TABLE gslb_switch_requests DROP COLUMN IF EXISTS dr_group_ref;
ALTER TABLE gslb_switch_requests DROP COLUMN IF EXISTS operation_id;

-- 恢复 008 的原始 operation_type CHECK；本特性产生的 gslb_* 行随回滚清除
-- （operation_steps 等经外键级联；operation_read_model 无外键，需显式清除）。
DELETE FROM operation_read_model WHERE operation_type IN (
    'gslb_failover', 'gslb_switchback', 'gslb_weight_update', 'gslb_drill'
);
DELETE FROM operations WHERE operation_type IN (
    'gslb_failover', 'gslb_switchback', 'gslb_weight_update', 'gslb_drill'
);
ALTER TABLE operations DROP CONSTRAINT IF EXISTS operations_operation_type_check;
ALTER TABLE operations ADD CONSTRAINT operations_operation_type_check CHECK (operation_type IN (
    'deploy', 'upgrade', 'rollback', 'scale', 'backup',
    'restore', 'switchover', 'delete', 'gc', 'ota', 'config_change'
));

COMMIT;
