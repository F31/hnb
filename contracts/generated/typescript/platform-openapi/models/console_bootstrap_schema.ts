/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { capability_schema } from './capability_schema';
import type { scoped_permission_schema } from './scoped_permission_schema';
export type console_bootstrap_schema = {
    subject: {
        id: string;
        type: 'user' | 'workload' | 'service';
        displayName: string;
    };
    selectedTenantId: string;
    memberships: Array<{
        membershipId: string;
        tenantId: string;
        tenantName: string;
    }>;
    capabilities: Array<capability_schema>;
    permissions: Array<scoped_permission_schema>;
    policyVersion: string;
    permissionVersion: string;
};
