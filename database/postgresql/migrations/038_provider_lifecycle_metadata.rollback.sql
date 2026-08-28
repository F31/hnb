-- Rollback: 038_provider_lifecycle_metadata
-- Development-only after proving no production lifecycle metadata is needed.

DROP TABLE IF EXISTS provider_navigation_metadata;
DROP TABLE IF EXISTS provider_capability_registrations;
DROP TABLE IF EXISTS provider_lifecycle_states;
