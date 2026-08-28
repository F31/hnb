/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type StorageOverview = {
    schemaVersion: string;
    source: string;
    observedAt: string;
    freshness: 'Fresh' | 'Stale' | 'Unknown';
    counts: {
        backends: number;
        offerings: number;
        driverInstallations: number;
        targets: number;
        bindings: number;
    };
    capacityStates: {
        Known: number;
        Elastic: number;
        Unknown: number;
        NotReported: number;
    };
};
