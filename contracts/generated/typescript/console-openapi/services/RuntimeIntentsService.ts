/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { ClusterRuntimeIntent } from '../models/ClusterRuntimeIntent';
import type { ProblemDetails } from '../models/ProblemDetails';
import type { RuntimeIntentSubmissionResponse } from '../models/RuntimeIntentSubmissionResponse';
import type { CancelablePromise } from '../core/CancelablePromise';
import { OpenAPI } from '../core/OpenAPI';
import { request as __request } from '../core/request';
export class RuntimeIntentsService {
    /**
     * Submit a typed cluster lifecycle intent
     * Header and body idempotency and correlation values must match. The server selects all Providers and Steps.
     * @returns ProblemDetails RFC 9457 problem with bounded, non-sensitive extensions.
     * @returns RuntimeIntentSubmissionResponse Intent accepted or exactly replayed; this is not an execution success.
     * @throws ApiError
     */
    public static submitClusterRuntimeIntent({
        xCorrelationId,
        idempotencyKey,
        requestBody,
    }: {
        xCorrelationId: string,
        idempotencyKey: string,
        requestBody: ClusterRuntimeIntent,
    }): CancelablePromise<ProblemDetails | RuntimeIntentSubmissionResponse> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/runtime-intents',
            headers: {
                'X-Correlation-ID': xCorrelationId,
                'Idempotency-Key': idempotencyKey,
            },
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                400: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                409: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                422: `RFC 9457 problem with bounded, non-sensitive extensions.`,
            },
        });
    }
}
