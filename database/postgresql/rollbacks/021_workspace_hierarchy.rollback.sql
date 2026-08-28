-- Rollback: 021_workspace_hierarchy
-- The additive hierarchy is intentionally retained. Dropping it would delete
-- 005-owned project/environment data or sever runtime target scope evidence.
SELECT 1;
