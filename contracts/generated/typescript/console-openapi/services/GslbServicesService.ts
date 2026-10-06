/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { GslbCreateServiceRequest } from '../models/GslbCreateServiceRequest';
import type { GslbDrillReportListResponse } from '../models/GslbDrillReportListResponse';
import type { GslbPool } from '../models/GslbPool';
import type { GslbPoolListResponse } from '../models/GslbPoolListResponse';
import type { GslbReadModel } from '../models/GslbReadModel';
import type { GslbReplacePoolsRequest } from '../models/GslbReplacePoolsRequest';
import type { GslbRuntimeIntent } from '../models/GslbRuntimeIntent';
import type { GslbServiceDetail } from '../models/GslbServiceDetail';
import type { GslbServiceListResponse } from '../models/GslbServiceListResponse';
import type { GslbSwitchRequest } from '../models/GslbSwitchRequest';
import type { GslbSwitchRequestListResponse } from '../models/GslbSwitchRequestListResponse';
import type { GslbSwitchStatus } from '../models/GslbSwitchStatus';
import type { GslbUpdateServiceRequest } from '../models/GslbUpdateServiceRequest';
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
     * Create a GSLB service with pools and health check template
     * GSLB 三期服务创建：校验 name/domain/pools/members（clusterId 必须匹配当前租户
     * runtime_targets 中的 Kubernetes 目标名称），priority 最小者为初始 active 池。幂等：同租户同 domain 重复创建返回
     * 既有服务（200）。Requires gslb:create.
     *
     * @returns GslbServiceDetail Idempotent replay — the existing service for this tenant+domain.
     * @returns ProblemDetails RFC 9457 problem with bounded, non-sensitive extensions.
     * @throws ApiError
     */
    public static createGslbService({
        xCorrelationId,
        idempotencyKey,
        requestBody,
    }: {
        xCorrelationId: string,
        idempotencyKey: string,
        requestBody: GslbCreateServiceRequest,
    }): CancelablePromise<GslbServiceDetail | ProblemDetails> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/gslb/services',
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
     * Update editable fields of a GSLB service
     * GSLB 四期服务编辑：仅 name / requireApproval / healthCheck 可变（domain、routingMode、
     * lifecycleState 不可由该路径修改）。FailingOver 中拒绝（409）。乐观并发：If-Match 携带
     * 先前读到的 updatedAt，过期返回 409。幂等：合并后与现状一致时返回 200 且不变更。
     * Requires gslb:update.
     *
     * @returns GslbServiceDetail The service after the update (unchanged on idempotent replay).
     * @returns ProblemDetails RFC 9457 problem with bounded, non-sensitive extensions.
     * @throws ApiError
     */
    public static updateGslbService({
        xCorrelationId,
        idempotencyKey,
        id,
        requestBody,
        ifMatch,
    }: {
        xCorrelationId: string,
        idempotencyKey: string,
        /**
         * GSLB service id.
         */
        id: string,
        requestBody: GslbUpdateServiceRequest,
        /**
         * Optimistic concurrency token for update/replace/delete operations: the resource's
         * updatedAt from a prior read (RFC 3339), or `*` to skip the version check.
         * A stale value returns 409.
         *
         */
        ifMatch?: string,
    }): CancelablePromise<GslbServiceDetail | ProblemDetails> {
        return __request(OpenAPI, {
            method: 'PATCH',
            url: '/api/v1/gslb/services/{id}',
            path: {
                'id': id,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
                'Idempotency-Key': idempotencyKey,
                'If-Match': ifMatch,
            },
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                400: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                401: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                403: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                404: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                409: `RFC 9457 problem with bounded, non-sensitive extensions.`,
            },
        });
    }
    /**
     * Delete a GSLB service (dangerous)
     * GSLB 四期服务删除：仅 lifecycleState 为 Inactive/Paused 可删；其余状态需显式
     * force=true，且服务当前不得有进行中（PendingApproval/Approved/Dispatched）的切换请求。
     * 删除经外键 CASCADE 同事务清理 pools/members/Read Model/切换历史/演练报告；
     * 关联的平台 operations 行保留作审计。Requires gslb:delete.
     *
     * @returns ProblemDetails RFC 9457 problem with bounded, non-sensitive extensions.
     * @throws ApiError
     */
    public static deleteGslbService({
        xCorrelationId,
        idempotencyKey,
        id,
        ifMatch,
        force = false,
    }: {
        xCorrelationId: string,
        idempotencyKey: string,
        /**
         * GSLB service id.
         */
        id: string,
        /**
         * Optimistic concurrency token for update/replace/delete operations: the resource's
         * updatedAt from a prior read (RFC 3339), or `*` to skip the version check.
         * A stale value returns 409.
         *
         */
        ifMatch?: string,
        /**
         * 强制删除非 Inactive/Paused 服务（仍要求无进行中切换请求）。
         */
        force?: boolean,
    }): CancelablePromise<ProblemDetails> {
        return __request(OpenAPI, {
            method: 'DELETE',
            url: '/api/v1/gslb/services/{id}',
            path: {
                'id': id,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
                'Idempotency-Key': idempotencyKey,
                'If-Match': ifMatch,
            },
            query: {
                'force': force,
            },
            errors: {
                400: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                401: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                403: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                404: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                409: `RFC 9457 problem with bounded, non-sensitive extensions.`,
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
     * List the pool topology of a GSLB service
     * 地址池拓扑（前端“地址池拓扑”页签）：isActive 对照服务 active_pool_id，
     * healthy 对照 Read Model healthy_pools（投影缺失时 false）。只读，请求路径零探测。
     *
     * @returns GslbPool Pools with members, ascending by priority.
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
     * Replace the whole pool topology of a GSLB service
     * GSLB 四期地址池整组替换：校验同创建（≥1 池、priority 唯一、成员非空、weight 0-100、
     * clusterId 必须匹配当前租户 runtime_targets 中的 Kubernetes 目标名称）。池以 name 为身份，服务端重新生成 pool id；
     * 当前 active 池的名字必须仍在新配置中，否则 409（先 failover 到保留的池）。
     * FailingOver 中拒绝（409）。事务内删旧建新并重指 active 池。幂等：新配置与现状语义
     * 一致时返回 200 且不变更。If-Match 乐观并发同 PATCH。Requires gslb:update.
     *
     * @returns GslbPoolListResponse The pool topology after replacement (unchanged on idempotent replay).
     * @returns ProblemDetails RFC 9457 problem with bounded, non-sensitive extensions.
     * @throws ApiError
     */
    public static replaceGslbServicePools({
        xCorrelationId,
        idempotencyKey,
        id,
        requestBody,
        ifMatch,
    }: {
        xCorrelationId: string,
        idempotencyKey: string,
        /**
         * GSLB service id.
         */
        id: string,
        requestBody: GslbReplacePoolsRequest,
        /**
         * Optimistic concurrency token for update/replace/delete operations: the resource's
         * updatedAt from a prior read (RFC 3339), or `*` to skip the version check.
         * A stale value returns 409.
         *
         */
        ifMatch?: string,
    }): CancelablePromise<GslbPoolListResponse | ProblemDetails> {
        return __request(OpenAPI, {
            method: 'PUT',
            url: '/api/v1/gslb/services/{id}/pools',
            path: {
                'id': id,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
                'Idempotency-Key': idempotencyKey,
                'If-Match': ifMatch,
            },
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                400: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                401: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                403: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                404: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                409: `RFC 9457 problem with bounded, non-sensitive extensions.`,
            },
        });
    }
    /**
     * List switch requests of a GSLB service
     * 切换记录/审批中心列表：租户隔离，service_id 必须指向当前租户的服务
     * （防越权枚举），按 created_at 倒序，上限 100 条。
     *
     * @returns GslbSwitchRequestListResponse Switch requests, newest first.
     * @returns ProblemDetails RFC 9457 problem with bounded, non-sensitive extensions.
     * @throws ApiError
     */
    public static listGslbSwitchRequests({
        xCorrelationId,
        serviceId,
        status,
    }: {
        xCorrelationId: string,
        /**
         * GSLB service id（必填，UUID）。
         */
        serviceId: string,
        /**
         * 精确匹配的状态过滤。
         */
        status?: GslbSwitchStatus,
    }): CancelablePromise<GslbSwitchRequestListResponse | ProblemDetails> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/gslb/switch-requests',
            headers: {
                'X-Correlation-ID': xCorrelationId,
            },
            query: {
                'service_id': serviceId,
                'status': status,
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
