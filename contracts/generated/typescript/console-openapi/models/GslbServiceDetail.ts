/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { GslbHealthCheck } from './GslbHealthCheck';
import type { GslbLifecycleState } from './GslbLifecycleState';
import type { GslbPool } from './GslbPool';
import type { GslbRoutingMode } from './GslbRoutingMode';
/**
 * 创建/详情响应：服务主档（含健康检查模板）+ 地址池拓扑。
 */
export type GslbServiceDetail = {
    id: string;
    tenantId: string;
    name: string;
    domain: string;
    routingMode: GslbRoutingMode;
    activePoolId?: string;
    lifecycleState: GslbLifecycleState;
    requireApproval: boolean;
    healthCheck?: GslbHealthCheck;
    pools: Array<GslbPool>;
    createdAt?: string;
    updatedAt?: string;
};
