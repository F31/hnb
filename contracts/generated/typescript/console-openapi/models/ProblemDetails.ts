/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type ProblemDetails = {
    type: string;
    title: string;
    status: number;
    detail?: string;
    instance?: string;
    code: 'VALIDATION_FAILED' | 'UNAUTHORIZED' | 'NOT_FOUND' | 'FORBIDDEN' | 'CONFLICT' | 'RATE_LIMITED' | 'INTERNAL_ERROR' | 'SERVICE_UNAVAILABLE' | 'UPSTREAM_UNAVAILABLE' | 'STALE_CONFIRMATION_REQUIRED' | 'STALE_CONFIRMATION_EXPIRED' | 'STALE_POLICY_DENIED' | 'IDEMPOTENCY_CONFLICT' | 'SECRET_REFERENCE_DENIED' | 'TARGET_VERSION_CONFLICT' | 'TARGET_ACTION_UNSUPPORTED' | 'PROVIDER_INCOMPATIBLE' | 'PROVIDER_ROUTE_NOT_FOUND' | 'OPERATION_ACTION_NOT_ALLOWED';
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
