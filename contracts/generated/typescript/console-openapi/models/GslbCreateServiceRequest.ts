/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { GslbHealthCheck } from './GslbHealthCheck';
import type { GslbPoolInput } from './GslbPoolInput';
export type GslbCreateServiceRequest = {
    name: string;
    /**
     * 租户内唯一；同租户同 domain 重复创建返回既有服务（200）。
     */
    domain: string;
    requireApproval: boolean;
    /**
     * priority 最小者为初始 active 池；priority 不可重复；clusterId 必须匹配当前租户 runtime_targets 中的 Kubernetes 目标名称。
     */
    pools: Array<GslbPoolInput>;
    healthCheck?: GslbHealthCheck;
};
