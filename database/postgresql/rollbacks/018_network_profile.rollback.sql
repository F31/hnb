-- Rollback: 018_network_profile
-- Description: Remove network_profiles and cilium_network_policies tables

DROP TABLE IF EXISTS cilium_network_policies;
DROP TABLE IF EXISTS network_profiles;