/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { GslbDrillReportListResponse } from '../models/GslbDrillReportListResponse';
import type { GslbPool } from '../models/GslbPool';
import type { GslbReadModel } from '../models/GslbReadModel';
import type { GslbRuntimeIntent } from '../models/GslbRuntimeIntent';
import type { GslbServiceListResponse } from '../models/GslbServiceListResponse';
import type { GslbSwitchRequest } from '../models/GslbSwitchRequest';
import type { ProblemDetails } from '../models/ProblemDetails';
import type { CancelablePromise } from '../core/CancelablePromise';
import { OpenAPI } from '../core/OpenAPI';
import { request as __request } from '../core/request';
export class GslbServicesService {
    /**
     * List tenant-scoped GSLB services from the Read Model
     * Reads gslb_read_model only (GSLB-007); never probes clusters or queries DNS on the request path.
     * @returns GslbServiceListResponse Tenant-scoped GSLB service projections.
     * @returns ProblemDetails RFC 9457 problem with bounded, non-sensitive extensions.
     * @throws ApiError
     */
    public static listGslbServices({
        xCorrelationId,
    }: {
        xCorrelationId: string,
    }): CancelablePromise<GslbServiceListResponse | ProblemDetails> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/gslb/services',
            headers: {
                'X-Correlation-ID': xCorrelationId,
            },
            errors: {
                400: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                401: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                403: `RFC 9457 problem with bounded, non-sensitive extensions.`,
            },
        });
    }
    /**
     * Get one GSLB service projection
     * @returns GslbReadModel GSLB service Read Model projection.
     * @returns ProblemDetails RFC 9457 problem with bounded, non-sensitive extensions.
     * @throws ApiError
     */
    public static getGslbService({
        xCorrelationId,
        id,
    }: {
        xCorrelationId: string,
        /**
         * GSLB service id.
         */
        id: string,
    }): CancelablePromise<GslbReadModel | ProblemDetails> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/gslb/services/{id}',
            path: {
                'id': id,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
            },
            errors: {
                400: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                401: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                403: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                404: `RFC 9457 problem with bounded, non-sensitive extensions.`,
            },
        });
    }
    /**
     * List structured drill reports of a GSLB service
     * GSLB-010 只读演练报告：结构化落库（gslb_drill_reports），按时间倒序返回。
     * 演练不产生任何真实 DNS 变更；查询只读落库报告，请求路径零探测。
     *
     * @returns GslbDrillReportListResponse Structured drill reports, newest first.
     * @returns ProblemDetails RFC 9457 problem with bounded, non-sensitive extensions.
     * @throws ApiError
     */
    public static listGslbDrillReports({
        xCorrelationId,
        id,
    }: {
        xCorrelationId: string,
        /**
         * GSLB service id.
         */
        id: string,
    }): CancelablePromise<GslbDrillReportListResponse | ProblemDetails> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/gslb/services/{id}/drills',
            path: {
                'id': id,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
            },
            errors: {
                400: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                401: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                403: `RFC 9457 problem with bounded, non-sensitive extensions.`,
            },
        });
    }
    /**
     * List the pools of a GSLB service
     * @returns GslbPool Pools with members.
     * @returns ProblemDetails RFC 9457 problem with bounded, non-sensitive extensions.
     * @throws ApiError
     */
    public static listGslbPools({
        xCorrelationId,
        id,
    }: {
        xCorrelationId: string,
        /**
         * GSLB service id.
         */
        id: string,
    }): CancelablePromise<Array<GslbPool> | ProblemDetails> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/gslb/services/{id}/pools',
            path: {
                'id': id,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
            },
            errors: {
                400: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                401: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                403: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                404: `RFC 9457 problem with bounded, non-sensitive extensions.`,
            },
        });
    }
    /**
     * Submit a controlled GSLB traffic change intent
     * GSLB-005 受控写路径：类型化 RuntimeIntent（gslb.failover / gslb.switchback /
     * gslb.weight-update / gslb.drill）经审批门控进入执行；意图不得携带执行细节。
     * failover/switchback 默认 require_approval。
     *
     * @returns GslbSwitchRequest The approval-gated switch request.
     * @returns ProblemDetails RFC 9457 problem with bounded, non-sensitive extensions.
     * @throws ApiError
     */
    public static submitGslbIntent({
        xCorrelationId,
        idempotencyKey,
        id,
        requestBody,
    }: {
        xCorrelationId: string,
        idempotencyKey: string,
        /**
         * GSLB service id.
         */
        id: string,
        requestBody: GslbRuntimeIntent,
    }): CancelablePromise<GslbSwitchRequest | ProblemDetails> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/gslb/services/{id}/intents',
            path: {
                'id': id,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
                'Idempotency-Key': idempotencyKey,
            },
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                400: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                401: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                403: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                404: `RFC 9457 problem with bounded, non-sensitive extensions.`,
            },
        });
    }
    /**
     * Approve a pending GSLB traffic change
     * Requires gslb:update. Approved executable requests are dispatched via outbox command.
     * @returns GslbSwitchRequest The approved switch request.
     * @returns ProblemDetails RFC 9457 problem with bounded, non-sensitive extensions.
     * @throws ApiError
     */
    public static approveGslbSwitchRequest({
        xCorrelationId,
        idempotencyKey,
        id,
    }: {
        xCorrelationId: string,
        idempotencyKey: string,
        /**
         * GSLB switch request id.
         */
        id: string,
    }): CancelablePromise<GslbSwitchRequest | ProblemDetails> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/gslb/switch-requests/{id}/approve',
            path: {
                'id': id,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
                'Idempotency-Key': idempotencyKey,
            },
            errors: {
                400: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                403: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                404: `RFC 9457 problem with bounded, non-sensitive extensions.`,
            },
        });
    }
    /**
     * Reject a pending GSLB traffic change
     * @returns GslbSwitchRequest The rejected switch request.
     * @returns ProblemDetails RFC 9457 problem with bounded, non-sensitive extensions.
     * @throws ApiError
     */
    public static rejectGslbSwitchRequest({
        xCorrelationId,
        idempotencyKey,
        id,
    }: {
        xCorrelationId: string,
        idempotencyKey: string,
        /**
         * GSLB switch request id.
         */
        id: string,
    }): CancelablePromise<GslbSwitchRequest | ProblemDetails> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/gslb/switch-requests/{id}/reject',
            path: {
                'id': id,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
                'Idempotency-Key': idempotencyKey,
            },
            errors: {
                400: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                403: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                404: `RFC 9457 problem with bounded, non-sensitive extensions.`,
            },
        });
    }
}
