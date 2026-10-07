-- 099 rollback: remove ONLY the seeded k8s version catalog; leave any
-- operator-managed value untouched.
BEGIN;

DELETE FROM platform_settings WHERE key = 'resource.cluster.kubernetesVersions';

COMMIT;