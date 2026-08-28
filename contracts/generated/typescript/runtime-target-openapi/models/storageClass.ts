/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { deleted } from './deleted';
import type { name } from './name';
import type { observedAt } from './observedAt';
import type { resourceVersion } from './resourceVersion';
import type { source } from './source';
import type { stringMap } from './stringMap';
import type { uid } from './uid';
export type storageClass = {
    uid: uid;
    resourceVersion: resourceVersion;
    name: name;
    source: source;
    observedAt: observedAt;
    deleted?: deleted;
    provisioner?: string;
    parameters?: stringMap;
    reclaimPolicy?: 'Delete' | 'Retain';
    volumeBindingMode?: 'Immediate' | 'WaitForFirstConsumer';
    allowVolumeExpansion?: boolean;
    isDefault?: boolean;
    mountOptions?: Array<string>;
    allowedTopologies?: Array<stringMap>;
};
