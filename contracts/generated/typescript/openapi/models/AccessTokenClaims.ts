/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { ScopedPermission } from './ScopedPermission';
export type AccessTokenClaims = {
    profileVersion: any;
    issuer: string;
    audiences: Array<string>;
    subjectId: string;
    subjectType: 'user' | 'workload' | 'service';
    tenantId: string;
    membershipId: string;
    tenantMembershipIds: Array<string>;
    policyVersion: string;
    scopedPermissions: Array<ScopedPermission>;
    allowedActions?: Array<'read' | 'list' | 'create' | 'update' | 'delete' | 'execute' | 'approve' | 'reject' | 'cancel' | 'switchTenant'>;
    issuedAt: string;
    expiresAt: string;
    notBefore: string;
    authTime: string;
    tokenId: string;
    keyId: string;
    algorithm: any;
};
