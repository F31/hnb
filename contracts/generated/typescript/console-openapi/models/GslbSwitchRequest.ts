/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { GslbIntentKind } from './GslbIntentKind';
export type GslbSwitchRequest = {
    id: string;
    tenantId: string;
    serviceId: string;
    intentKind: GslbIntentKind;
    intentDigest: string;
    idempotencyKey?: string;
    correlationId?: string;
    requireApproval: boolean;
    status: 'PendingApproval' | 'Approved' | 'Rejected' | 'Dispatched' | 'Succeeded' | 'Failed' | 'DrillCompleted';
    actorId?: string;
    approvedBy?: string;
    approvedAt?: string;
    reason?: string;
    error?: string;
    /**
     * 关联的平台 operations 行（Operation Center 统一接线）。
     */
    operationId?: string;
    /**
     * DRProtectionGroup 编排来源引用（GSLB-009）。
     */
    drGroupRef?: string;
    createdAt?: string;
    updatedAt?: string;
};
