-- 099: kubernetes_version_catalog_seed
-- Seed the platform-managed Kubernetes version catalog used by the
-- "平台创建" wizard. The catalog is stored under platform_settings key
-- resource.cluster.kubernetesVersions so operators can add/retire versions
-- ("按需配置") without rebuilding the console. Each entry records:
--   integrationTestStatus : passed | pending | blocked  (平台集成测试情况)
--   offlineBundleAvailable: whether a kubeadm/bootstrap offline bundle exists
-- Configuring a version here is necessary but not sufficient: the
-- runtime-target-lifecycle-provider must also accept it as a bootstrap
-- target (DesiredVersion validation).

BEGIN;

INSERT INTO platform_settings (key, value, updated_at) VALUES (
    'resource.cluster.kubernetesVersions',
    '{
      "items": [
      ]
    }'::jsonb,
    now()
)
ON CONFLICT (key) DO NOTHING;

COMMIT;
