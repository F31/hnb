/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { csiNodeDriver } from './csiNodeDriver';
import type { deleted } from './deleted';
import type { name } from './name';
import type { observedAt } from './observedAt';
import type { resourceVersion } from './resourceVersion';
import type { source } from './source';
import type { uid } from './uid';
export type csiNode = {
    uid: uid;
    resourceVersion: resourceVersion;
    name: name;
    source: source;
    observedAt: observedAt;
    deleted?: deleted;
    drivers?: Array<csiNodeDriver>;
};
