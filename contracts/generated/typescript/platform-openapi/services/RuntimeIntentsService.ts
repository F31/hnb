/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { problem_details_schema } from '../models/problem_details_schema';
import type { runtime_intent_schema } from '../models/runtime_intent_schema';
import type { RuntimeIntentRecord } from '../models/RuntimeIntentRecord';
import type { CancelablePromise } from '../core/CancelablePromise';
import { OpenAPI } from '../core/OpenAPI';
import { request as __request } from '../core/request';
export class RuntimeIntentsService {
    /**
     * Submit a typed runtime mutation intent for server-side planning
     * @returns problem_details_schema RFC 9457 problem response.
     * @returns RuntimeIntentRecord Intent was accepted for validation and immutable planning.
     * @throws ApiError
     */
    public static submitRuntimeIntent({
        xCorrelationId,
        idempotencyKey,
        requestBody,
    }: {
        xCorrelationId: string,
        idempotencyKey: string,
        requestBody: runtime_intent_schema,
    }): CancelablePromise<problem_details_schema | RuntimeIntentRecord> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/v1/runtime-intents',
            headers: {
                'X-Correlation-ID': xCorrelationId,
                'Idempotency-Key': idempotencyKey,
            },
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                400: `RFC 9457 problem response.`,
            },
        });
    }
    /**
     * Get a tenant-scoped runtime intent and canonical operation linkage
     * @returns RuntimeIntentRecord Current immutable intent record and canonical linkage.
     * @returns problem_details_schema RFC 9457 problem response.
     * @throws ApiError
     */
    public static getRuntimeIntent({
        intentId,
        xCorrelationId,
    }: {
        intentId: string,
        xCorrelationId: string,
    }): CancelablePromise<RuntimeIntentRecord | problem_details_schema> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/v1/runtime-intents/{intentId}',
            path: {
                'intentId': intentId,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
            },
            errors: {
                404: `RFC 9457 problem response.`,
            },
        });
    }
}
