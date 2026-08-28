/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type problem_details_schema = {
    type: string;
    title: string;
    status: number;
    detail?: string;
    instance?: string;
    code: string;
    correlationId: string;
    traceId: string;
    retryable?: boolean;
    confirmation?: string;
    targetId?: string;
    action?: string;
    lastKnownStateAt?: string;
    lifecycleState?: string;
    healthState?: string;
    connectivityState?: string;
    policyOutcome?: 'allow' | 'require_approval' | 'queued_offline' | 'deny';
    violations?: Array<{
        field: string;
        code: string;
        message?: string;
    }>;
};
