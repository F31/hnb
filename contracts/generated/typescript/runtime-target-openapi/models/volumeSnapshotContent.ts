/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { deleted } from './deleted';
import type { name } from './name';
import type { observedAt } from './observedAt';
import type { resourceVersion } from './resourceVersion';
import type { source } from './source';
import type { uid } from './uid';
export type volumeSnapshotContent = {
    uid: uid;
    resourceVersion: resourceVersion;
    name: name;
    source: source;
    observedAt: observedAt;
    deleted?: deleted;
    driver?: string;
    deletionPolicy?: 'Delete' | 'Retain';
    snapshotHandle?: string;
    volumeSnapshotNamespace?: string;
    volumeSnapshotName?: string;
    readyToUse?: boolean;
    restoreSizeBytes?: number;
    error?: string;
};
