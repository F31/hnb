/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { GslbDrillCheck } from './GslbDrillCheck';
import type { GslbDrillVerdict } from './GslbDrillVerdict';
export type GslbDrillReportPayload = {
    serviceId: string;
    domain: string;
    activePoolId?: string;
    targetPoolId?: string;
    currentTargets: Array<string>;
    projectedTargets: Array<string>;
    projectedWeights?: Record<string, number>;
    healthyPools?: Array<string>;
    checks: Array<GslbDrillCheck>;
    verdict: GslbDrillVerdict;
    generatedAt: string;
};
