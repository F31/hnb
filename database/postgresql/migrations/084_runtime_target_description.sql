-- Migration: 084_runtime_target_description
-- Description: Add an optional human-readable description to runtime_targets,
-- editable via the console (cluster detail > description).
ALTER TABLE runtime_targets ADD COLUMN IF NOT EXISTS description TEXT;
