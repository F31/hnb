/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { ClusterIntentKind } from '../models/ClusterIntentKind';
import type { OperationActionRequest } from '../models/OperationActionRequest';
import type { OperationActionsResponse } from '../models/OperationActionsResponse';
import type { OperationDetailResponse } from '../models/OperationDetailResponse';
import type { OperationListResponse } from '../models/OperationListResponse';
import type { OperationStatus } from '../models/OperationStatus';
import type { ProblemDetails } from '../models/ProblemDetails';
import type { CancelablePromise } from '../core/CancelablePromise';
import { OpenAPI } from '../core/OpenAPI';
import { request as __request } from '../core/request';
export class OperationsService {
    /**
     * List tenant-scoped Operations
     * @returns OperationListResponse Exact tenant-scoped Operation page.
     * @returns ProblemDetails RFC 9457 problem with bounded, non-sensitive extensions.
     * @throws ApiError
     */
    public static listOperations({
        xCorrelationId,
        page = 1,
        pageSize = 20,
        status,
        type,
        targetId,
    }: {
        xCorrelationId: string,
        page?: number,
        pageSize?: number,
        status?: OperationStatus,
        type?: ClusterIntentKind,
        targetId?: string,
    }): CancelablePromise<OperationListResponse | ProblemDetails> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/operations',
            headers: {
                'X-Correlation-ID': xCorrelationId,
            },
            query: {
                'page': page,
                'pageSize': pageSize,
                'status': status,
                'type': type,
                'targetId': targetId,
            },
            errors: {
                400: `RFC 9457 problem with bounded, non-sensitive extensions.`,
            },
        });
    }
    /**
     * Get an Operation projection
     * @returns OperationDetailResponse Operation detail, progress, safe failure, deep links, and allowed actions.
     * @returns ProblemDetails RFC 9457 problem with bounded, non-sensitive extensions.
     * @throws ApiError
     */
    public static getOperation({
        xCorrelationId,
        operationId,
    }: {
        xCorrelationId: string,
        operationId: string,
    }): CancelablePromise<OperationDetailResponse | ProblemDetails> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/operations/{operationId}',
            path: {
                'operationId': operationId,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
            },
            errors: {
                404: `RFC 9457 problem with bounded, non-sensitive extensions.`,
            },
        });
    }
    /**
     * Get actions allowed for the current actor and Operation state
     * @returns OperationActionsResponse Current actor-specific action projection.
     * @returns ProblemDetails RFC 9457 problem with bounded, non-sensitive extensions.
     * @throws ApiError
     */
    public static listOperationActions({
        xCorrelationId,
        operationId,
    }: {
        xCorrelationId: string,
        operationId: string,
    }): CancelablePromise<OperationActionsResponse | ProblemDetails> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/operations/{operationId}/actions',
            path: {
                'operationId': operationId,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
            },
            errors: {
                404: `RFC 9457 problem with bounded, non-sensitive extensions.`,
            },
        });
    }
    /**
     * Approve an Operation through the authoritative domain API
     * @returns OperationDetailResponse Authoritative action result and refreshed Operation projection.
     * @returns ProblemDetails RFC 9457 problem with bounded, non-sensitive extensions.
     * @throws ApiError
     */
    public static approveOperation({
        xCorrelationId,
        idempotencyKey,
        operationId,
        requestBody,
    }: {
        xCorrelationId: string,
        idempotencyKey: string,
        operationId: string,
        requestBody?: OperationActionRequest,
    }): CancelablePromise<OperationDetailResponse | ProblemDetails> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/operations/{operationId}/actions/approve',
            path: {
                'operationId': operationId,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
                'Idempotency-Key': idempotencyKey,
            },
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                409: `RFC 9457 problem with bounded, non-sensitive extensions.`,
            },
        });
    }
    /**
     * Reject an Operation through the authoritative domain API
     * @returns OperationDetailResponse Authoritative action result and refreshed Operation projection.
     * @returns ProblemDetails RFC 9457 problem with bounded, non-sensitive extensions.
     * @throws ApiError
     */
    public static rejectOperation({
        xCorrelationId,
        idempotencyKey,
        operationId,
        requestBody,
    }: {
        xCorrelationId: string,
        idempotencyKey: string,
        operationId: string,
        requestBody?: OperationActionRequest,
    }): CancelablePromise<OperationDetailResponse | ProblemDetails> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/operations/{operationId}/actions/reject',
            path: {
                'operationId': operationId,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
                'Idempotency-Key': idempotencyKey,
            },
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                409: `RFC 9457 problem with bounded, non-sensitive extensions.`,
            },
        });
    }
    /**
     * Cancel an Operation through the authoritative domain API
     * @returns OperationDetailResponse Authoritative action result and refreshed Operation projection.
     * @returns ProblemDetails RFC 9457 problem with bounded, non-sensitive extensions.
     * @throws ApiError
     */
    public static cancelOperation({
        xCorrelationId,
        idempotencyKey,
        operationId,
        requestBody,
    }: {
        xCorrelationId: string,
        idempotencyKey: string,
        operationId: string,
        requestBody?: OperationActionRequest,
    }): CancelablePromise<OperationDetailResponse | ProblemDetails> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/operations/{operationId}/actions/cancel',
            path: {
                'operationId': operationId,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
                'Idempotency-Key': idempotencyKey,
            },
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                409: `RFC 9457 problem with bounded, non-sensitive extensions.`,
            },
        });
    }
}
