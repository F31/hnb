-- 074: storage_volume_semantics_boundary
-- Description: Make workload-volume and StorageClass-only semantics explicit in storage desired state.
-- Dependencies: 073_storage_driver_package_manifest

BEGIN;

ALTER TABLE workload_storage_offerings
    ADD COLUMN IF NOT EXISTS consumption_model TEXT NOT NULL DEFAULT 'KubernetesPersistentVolume'
    CHECK (consumption_model = 'KubernetesPersistentVolume');

ALTER TABLE storage_class_bindings
    ADD COLUMN IF NOT EXISTS binding_target TEXT NOT NULL DEFAULT 'KubernetesStorageClass'
    CHECK (binding_target = 'KubernetesStorageClass');

COMMIT;
