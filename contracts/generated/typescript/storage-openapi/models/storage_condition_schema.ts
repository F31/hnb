/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type storage_condition_schema = {
    schemaVersion: string;
    type: string;
    status: 'True' | 'False' | 'Unknown';
    reason: string;
    message?: string;
    source: string;
    observedAt: string;
    freshness: 'Fresh' | 'Stale' | 'Unknown';
};
