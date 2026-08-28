/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { DrRunStatus } from './DrRunStatus';
import type { DrSwitchDirection } from './DrSwitchDirection';
export type DrSwitchRun = {
    id: string;
    groupId: string;
    tenantId: string;
    direction: DrSwitchDirection;
    status: DrRunStatus;
    idempotencyKey: string;
    correlationId: string;
    /**
     * 关联的平台 operations 行（Operation Center 统一接线）。
     */
    operationId?: string;
    /**
     * 流量层子 gslb 切换请求 id 列表（每 gslb_service 成员一个）。
     */
    trafficRequestIds: Array<string>;
    reason?: string;
    error?: string;
    actorId?: string;
    createdAt?: string;
    updatedAt?: string;
};
