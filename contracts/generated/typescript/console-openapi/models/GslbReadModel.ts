/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { GslbDrillVerdict } from './GslbDrillVerdict';
import type { GslbLifecycleState } from './GslbLifecycleState';
export type GslbReadModel = {
    serviceId: string;
    tenantId: string;
    domain: string;
    activePoolId?: string;
    lifecycleState: GslbLifecycleState;
    healthyPools?: Array<string>;
    currentDnsTargets?: Array<string>;
    lastSwitchRequestId?: string;
    lastSwitchAt?: string;
    lastDrillReportId?: string;
    lastDrillVerdict?: GslbDrillVerdict;
    lastDrillAt?: string;
    observedAt: string;
};
