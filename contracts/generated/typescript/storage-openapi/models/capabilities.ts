/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { supportClaim } from './supportClaim';
export type capabilities = {
    volumeModes: Array<'Block' | 'File'>;
    accessModes: Array<'ReadWriteOnce' | 'ReadOnlyMany' | 'ReadWriteMany' | 'ReadWriteOncePod'>;
    topology: supportClaim;
    capacityTracking: Array<'CSIStorageCapacity' | 'Provider' | 'NodeInventory' | 'None'>;
    expansion: supportClaim;
    clone: supportClaim;
    snapshot: supportClaim;
    ephemeral: supportClaim;
    health: supportClaim;
};
