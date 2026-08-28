/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
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
    createdAt?: string;
    updatedAt?: string;
};
