/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type retained_volume_workflow_result_schema = {
    schemaVersion: string;
    state: 'Sanitized' | 'ManualReleaseRequired';
    volumeId: string;
    providerId: string;
    operationId: string;
    stepId: string;
    idempotencyKey: string;
    fencingGeneration: number;
    recordedAt: string;
    sanitizationEvidence?: {
        evidenceRef: string;
        evidenceDigest: string;
        method: string;
        completedAt: string;
    };
    manualRelease?: {
        reason: string;
        dataRetained: boolean;
    };
};
