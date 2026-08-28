/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { csiDriver } from './csiDriver';
import type { csiNode } from './csiNode';
import type { csiStorageCapacity } from './csiStorageCapacity';
import type { storageClass } from './storageClass';
import type { volumeAttachment } from './volumeAttachment';
import type { volumeSnapshot } from './volumeSnapshot';
import type { volumeSnapshotClass } from './volumeSnapshotClass';
import type { volumeSnapshotContent } from './volumeSnapshotContent';
/**
 * Bounded Kubernetes storage facts. inventoryMode, observerGeneration, and sequence are inherited from the enclosing RuntimeTargetObservation.
 */
export type storage_inventory_section_schema = {
    storageClasses?: Array<storageClass>;
    csiDrivers?: Array<csiDriver>;
    csiNodes?: Array<csiNode>;
    csiStorageCapacities?: Array<csiStorageCapacity>;
    volumeAttachments?: Array<volumeAttachment>;
    volumeSnapshotClasses?: Array<volumeSnapshotClass>;
    volumeSnapshots?: Array<volumeSnapshot>;
    volumeSnapshotContents?: Array<volumeSnapshotContent>;
    snapshotApi?: {
        status: 'Installed' | 'NotInstalled' | 'Unsupported';
        apiVersion?: string;
        source: string;
        observedAt: string;
    };
};
