-- 061: drop_allowed_environments
-- Remove legacy allowed_environments column from entitlements
-- (environment concept was removed in hierarchy simplification)

ALTER TABLE IF EXISTS entitlements DROP COLUMN IF EXISTS allowed_environments;