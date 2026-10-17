/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type ClusterAcceptance = {
    /**
     * The platform accepted the target/import request; this alone does not mean it is available.
     */
    accepted: boolean;
    /**
     * True only when the target is ACTIVE, HEALTHY, CONNECTED and freshly observed.
     */
    available: boolean;
    observationState: 'pending' | 'observed' | 'stale';
    acceptedAt?: string;
    observationGeneration?: number;
    observationRevision?: number;
    observationSource?: 'agent' | 'cloudcore';
};
