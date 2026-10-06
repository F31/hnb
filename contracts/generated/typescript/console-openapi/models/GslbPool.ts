/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { GslbPoolMember } from './GslbPoolMember';
export type GslbPool = {
    id: string;
    /**
     * Alias of id for newer UI code; legacy clients should use id.
     */
    poolId?: string;
    serviceId: string;
    name: string;
    priority: number;
    /**
     * 是否为服务当前活跃池（对照 gslb_services.active_pool_id）。
     */
    isActive: boolean;
    /**
     * 是否在 Read Model 健康投影中（投影缺失时 false）。
     */
    healthy: boolean;
    members: Array<GslbPoolMember>;
};
