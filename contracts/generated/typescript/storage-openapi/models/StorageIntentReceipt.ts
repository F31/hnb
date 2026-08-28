/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type StorageIntentReceipt = {
    intentId: string;
    executionPlanId: string;
    operationId: string;
    status: 'planned' | 'operationCommitted' | 'rejected';
    semanticDigest: string;
    createdAt: string;
    correlationId: string;
    replayed: boolean;
};
