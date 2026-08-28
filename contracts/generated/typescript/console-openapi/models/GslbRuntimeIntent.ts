/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { GslbIntentKind } from './GslbIntentKind';
export type GslbRuntimeIntent = {
    apiVersion: string;
    kind: GslbIntentKind;
    serviceId: string;
    tenantId: string;
    targetPoolId?: string;
    weights?: Record<string, number>;
    reason?: string;
    /**
     * DRProtectionGroup 编排来源引用（GSLB-009 对接缝）；DR 来源的回切强制 require_approval。
     */
    drGroupRef?: string;
    metadata: {
        idempotencyKey: string;
        correlationId: string;
    };
};
