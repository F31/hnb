/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { ApiVersion } from './ApiVersion';
import type { OperationLinks } from './OperationLinks';
export type RuntimeIntentSubmissionResponse = {
    apiVersion: ApiVersion;
    intentId: string;
    executionPlanId: string;
    operationId: string;
    status: 'accepted' | 'pending_approval' | 'queued' | 'queued_offline';
    replayed: boolean;
    correlationId: string;
    links: OperationLinks;
};
