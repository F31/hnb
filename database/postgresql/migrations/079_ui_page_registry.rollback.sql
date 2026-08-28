-- Migration: 079_ui_page_registry (rollback)
-- Description: Drop the UI Registry PageSchema tables.

BEGIN;

DROP TABLE IF EXISTS ui_page_versions;
DROP TABLE IF EXISTS ui_pages;

COMMIT;
