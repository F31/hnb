/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { retainedVolumeResource } from './retainedVolumeResource';
import type { secret_reference_schema } from './secret_reference_schema';
export type runtime_intent_schema = {
    apiVersion: any;
    kind: 'InstallRelease' | 'UninstallRelease' | 'UpgradeRelease' | 'RollbackRelease' | 'ChangeConfiguration' | 'CreateKubernetesTarget' | 'ImportRuntimeTarget' | 'UpgradeRuntimeTarget' | 'DeleteRuntimeTarget' | 'ImportStorageClassBinding' | 'ReconcileStorageClassBinding' | 'InstallStorageDriver' | 'UpgradeStorageDriver' | 'UninstallStorageDriver' | 'ReleaseRetainedVolume' | 'SanitizeRetainedVolume';
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
        parameters?: Record<string, (string | number | boolean | null)>;
        secretReferences?: Array<secret_reference_schema>;
    };
};
