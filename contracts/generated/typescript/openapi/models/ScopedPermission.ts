/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type ScopedPermission = {
    resourceKind: string;
    resourceId?: string;
    action: 'read' | 'list' | 'create' | 'update' | 'delete' | 'execute' | 'approve' | 'reject' | 'cancel' | 'switchTenant';
    tenantId: string;
    projectId?: string;
    environmentId?: string;
    namespaceId?: string;
};
