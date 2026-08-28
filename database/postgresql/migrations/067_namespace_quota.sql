-- 067: namespace_quota
-- Description: Add unified resource quota to namespaces (CPU/memory/storage/vGPU/VRAM/GPU).
--   The quota is stored as a JSONB column consistent with tenants and workspaces
--   (see 056_tenant_workspace_quota.sql), driven by the shared core.Quota model.

BEGIN;

ALTER TABLE namespaces ADD COLUMN IF NOT EXISTS quota JSONB NOT NULL DEFAULT '{}';

COMMIT;