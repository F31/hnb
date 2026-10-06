/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { GslbHealthCheck } from './GslbHealthCheck';
import type { GslbLifecycleState } from './GslbLifecycleState';
import type { GslbRoutingMode } from './GslbRoutingMode';
export type GslbService = {
    id: string;
    tenantId: string;
    name: string;
    domain: string;
    routingMode: GslbRoutingMode;
    activePoolId?: string;
    lifecycleState: GslbLifecycleState;
    requireApproval: boolean;
    healthCheck?: GslbHealthCheck;
    createdAt?: string;
    updatedAt?: string;
};
