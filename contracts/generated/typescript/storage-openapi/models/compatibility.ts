/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { semanticVersion } from './semanticVersion';
import type { versionRange } from './versionRange';
export type compatibility = {
    kubernetesVersions: Array<versionRange>;
    upgradeFromVersions: Array<semanticVersion>;
    rollbackToVersions: Array<semanticVersion>;
};
