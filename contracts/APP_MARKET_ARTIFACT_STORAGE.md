# App Market Artifact Storage Contract

App Market owns artifact descriptor, storage profile, distribution and GC metadata. Platform and providers consume versioned HTTP APIs and event/Operation payloads containing IDs, digests and status only.

Platform MUST NOT query App Market tables directly. Providers MUST NOT receive robot tokens, secret values or artifact bytes through control-plane events.

Contract file: `contracts/openapi/app-market/v1/openapi.yaml`.

## Workload Storage Boundary

- `ArtifactStorageProfile` remains an App Market model and is persisted only in App Market-owned `artifact_storage_profiles` tables.
- Workload storage APIs and tables MUST NOT create, list, update, delete, or reference an `ArtifactStorageProfile`.
- A `WorkloadStorageOffering` always has `consumptionModel: KubernetesPersistentVolume`; object/bucket APIs such as S3 and MinIO are not valid offering semantics.
- A `StorageClassBinding` always has `bindingTarget: KubernetesStorageClass`; bucket names, endpoints, profile IDs, and object-service connectors are not binding identities.
- A provider that implements a separately versioned, conformant bucket-service contract exposes it through its own Connector/API and storage, IAM, and table ownership. That contract does not reuse ordinary volume endpoints or tables.
