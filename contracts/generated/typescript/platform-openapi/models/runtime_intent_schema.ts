/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { retainedVolumeResource } from './retainedVolumeResource';
import type { secret_reference_schema } from './secret_reference_schema';
import type { workerNodes } from './workerNodes';
export type runtime_intent_schema = {
    apiVersion: any;
    kind: 'InstallRelease' | 'UninstallRelease' | 'UpgradeRelease' | 'RollbackRelease' | 'ChangeConfiguration' | 'CreateKubernetesTarget' | 'ImportRuntimeTarget' | 'UpgradeRuntimeTarget' | 'DeleteRuntimeTarget' | 'AddNodesRuntimeTarget' | 'ImportStorageClassBinding' | 'ReconcileStorageClassBinding' | 'InstallStorageDriver' | 'UpgradeStorageDriver' | 'UninstallStorageDriver' | 'ReleaseRetainedVolume' | 'SanitizeRetainedVolume';
    metadata: {
        idempotencyKey: string;
        correlationId: string;
    };
    spec: {
        releaseId?: string;
        bindingId?: string;
        bindingVersion?: number;
        offeringId?: string;
        offeringVersion?: number;
        targetId?: string;
        targetKind?: any;
        expectedVersion?: number;
        storageClassName?: string;
        storageClassUid?: string;
        storageClassResourceVersion?: string;
        installationId?: string;
        packageId?: string;
        packageVersion?: string;
        currentVersion?: string;
        kubernetesVersion?: string;
        volumeId?: string;
        workflowProviderRef?: string;
        persistentVolume?: retainedVolumeResource;
        persistentVolumeClaim?: retainedVolumeResource;
        podDependencies?: any[];
        statefulSetDependencies?: any[];
        targetRef?: string;
        scopeRef?: string;
        parameters?: {
            nodes?: workerNodes;
        };
        secretReferences?: Array<secret_reference_schema>;
        credentialSecretRef?: secret_reference_schema;
        sshSecretRef?: secret_reference_schema;
    };
};
