/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { DrAddMemberRequest } from '../models/DrAddMemberRequest';
import type { DrCreateGroupRequest } from '../models/DrCreateGroupRequest';
import type { DrGroupDetailResponse } from '../models/DrGroupDetailResponse';
import type { DrGroupListResponse } from '../models/DrGroupListResponse';
import type { DrGroupMember } from '../models/DrGroupMember';
import type { DrInitiateSwitchRequest } from '../models/DrInitiateSwitchRequest';
import type { DrProtectionGroup } from '../models/DrProtectionGroup';
import type { DrSwitchRun } from '../models/DrSwitchRun';
import type { DrSwitchRunListResponse } from '../models/DrSwitchRunListResponse';
import type { ProblemDetails } from '../models/ProblemDetails';
import type { CancelablePromise } from '../core/CancelablePromise';
import { OpenAPI } from '../core/OpenAPI';
import { request as __request } from '../core/request';
export class DrProtectionGroupsService {
    /**
     * List tenant-scoped DR protection groups
     * OBS-008 只读列表；DR 保护组按“数据层 → 流量层”顺序编排地域级切换。
     * @returns DrGroupListResponse Tenant-scoped DR protection groups.
     * @returns ProblemDetails RFC 9457 problem with bounded, non-sensitive extensions.
     * @throws ApiError
     */
    public static listDrGroups({
        xCorrelationId,
    }: {
        xCorrelationId: string,
    }): CancelablePromise<DrGroupListResponse | ProblemDetails> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/dr/groups',
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
     * Create a DR protection group
     * Requires dr:create. The group starts in lifecycleState Ready.
     * @returns ProblemDetails RFC 9457 problem with bounded, non-sensitive extensions.
     * @returns DrProtectionGroup The created DR protection group.
     * @throws ApiError
     */
    public static createDrGroup({
        xCorrelationId,
        idempotencyKey,
        requestBody,
    }: {
        xCorrelationId: string,
        idempotencyKey: string,
        requestBody: DrCreateGroupRequest,
    }): CancelablePromise<ProblemDetails | DrProtectionGroup> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/dr/groups',
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
     * Get one DR protection group with members and recent runs
     * @returns DrGroupDetailResponse DR protection group detail.
     * @returns ProblemDetails RFC 9457 problem with bounded, non-sensitive extensions.
     * @throws ApiError
     */
    public static getDrGroup({
        xCorrelationId,
        id,
    }: {
        xCorrelationId: string,
        /**
         * DR protection group id.
         */
        id: string,
    }): CancelablePromise<DrGroupDetailResponse | ProblemDetails> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/dr/groups/{id}',
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
     * Add a member to a DR protection group
     * Requires dr:update. gslb_service 成员为流量层切换目标（引用必须存在）；
     * data_layer_ref 为数据层引用，切换时停在 DataLayerPending 等待人工确认门。
     *
     * @returns ProblemDetails RFC 9457 problem with bounded, non-sensitive extensions.
     * @returns DrGroupMember The added group member.
     * @throws ApiError
     */
    public static addDrGroupMember({
        xCorrelationId,
        idempotencyKey,
        id,
        requestBody,
    }: {
        xCorrelationId: string,
        idempotencyKey: string,
        /**
         * DR protection group id.
         */
        id: string,
        requestBody: DrAddMemberRequest,
    }): CancelablePromise<ProblemDetails | DrGroupMember> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/dr/groups/{id}/members',
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
     * List switch runs of a DR protection group
     * 终态按子 gslb 切换请求聚合推导（全部 Succeeded → Completed；任一 Failed/Rejected → Failed）。
     * @returns DrSwitchRunListResponse Switch runs of the group.
     * @returns ProblemDetails RFC 9457 problem with bounded, non-sensitive extensions.
     * @throws ApiError
     */
    public static listDrSwitchRuns({
        xCorrelationId,
        id,
    }: {
        xCorrelationId: string,
        /**
         * DR protection group id.
         */
        id: string,
    }): CancelablePromise<DrSwitchRunListResponse | ProblemDetails> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/dr/groups/{id}/runs',
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
     * Initiate a DR switch chain (failover or switchback)
     * Requires dr:execute. 幂等创建运行：同 idempotencyKey 直接返回既有运行；
     * 单活跃运行守卫，存在进行中运行返回 409。无数据层成员时立即进入流量层，
     * 否则停留 DataLayerPending 等待 confirm-data-layer 显式确认（OBS-008 顺序保证）。
     * 流量层步骤复用 gslb 受控意图链路（drGroupRef + 审批门控）。
     *
     * @returns DrSwitchRun The switch run (existing run returned on idempotent retry).
     * @returns ProblemDetails RFC 9457 problem with bounded, non-sensitive extensions.
     * @throws ApiError
     */
    public static initiateDrSwitch({
        xCorrelationId,
        idempotencyKey,
        id,
        requestBody,
    }: {
        xCorrelationId: string,
        idempotencyKey: string,
        /**
         * DR protection group id.
         */
        id: string,
        requestBody: DrInitiateSwitchRequest,
    }): CancelablePromise<DrSwitchRun | ProblemDetails> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/dr/groups/{id}/switch',
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
                409: `RFC 9457 problem with bounded, non-sensitive extensions.`,
            },
        });
    }
    /**
     * Confirm the data-layer step of a switch run
     * Requires dr:update. 仅 DataLayerPending 状态的运行可确认；确认后推进流量层步骤。
     *
     * @returns DrSwitchRun The switch run after data-layer confirmation.
     * @returns ProblemDetails RFC 9457 problem with bounded, non-sensitive extensions.
     * @throws ApiError
     */
    public static confirmDrDataLayer({
        xCorrelationId,
        idempotencyKey,
        id,
    }: {
        xCorrelationId: string,
        idempotencyKey: string,
        /**
         * DR switch run id.
         */
        id: string,
    }): CancelablePromise<DrSwitchRun | ProblemDetails> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/dr/runs/{id}/confirm-data-layer',
            path: {
                'id': id,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
                'Idempotency-Key': idempotencyKey,
            },
            errors: {
                400: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                401: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                403: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                404: `RFC 9457 problem with bounded, non-sensitive extensions.`,
            },
        });
    }
}
