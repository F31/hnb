/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type WorkloadStorageOfferingInput = {
    backendId?: string;
    name: string;
    description?: string;
    consumptionModel: string;
    serviceMode: 'Block' | 'File';
    accessModes: Array<'ReadWriteOnce' | 'ReadOnlyMany' | 'ReadWriteMany' | 'ReadWriteOncePod'>;
    volumeExpansion: 'Supported' | 'Unsupported' | 'Unknown';
    snapshots: 'Supported' | 'Unsupported' | 'Unknown';
    clones: 'Supported' | 'Unsupported' | 'Unknown';
    topology?: Record<string, Array<string>>;
    protectionClass: string;
};
