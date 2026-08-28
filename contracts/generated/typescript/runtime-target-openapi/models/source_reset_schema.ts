/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type source_reset_schema = {
    schemaVersion: string;
    eventId: string;
    tenantId: string;
    targetId: string;
    targetKind: 'KubernetesTarget' | 'EdgeRuntimeTarget';
    observerId: string;
    observerKind: 'Agent' | 'CloudCore';
    previousObserverGeneration: number;
    newObserverGeneration: number;
    observedAt: string;
    observerLeaseId: string;
    reason: 'observer-restarted' | 'lease-reissued' | 'disaster-recovery';
};
