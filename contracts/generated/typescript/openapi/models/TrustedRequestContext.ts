/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { ScopedPermission } from './ScopedPermission';
export type TrustedRequestContext = {
    subjectId: string;
    subjectType: 'user' | 'workload' | 'service';
    tenantId: string;
    membershipId: string;
    policyVersion: string;
    scopedPermissions: Array<ScopedPermission>;
    projectId?: string;
    environmentId?: string;
    namespaceId?: string;
    correlationId: string;
    traceparent?: string;
    tokenId: string;
    authTime: string;
};
