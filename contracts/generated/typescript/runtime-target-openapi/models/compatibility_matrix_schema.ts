/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type compatibility_matrix_schema = {
    schemaVersion: string;
    matrixVersion: string;
    providerProtocolVersion: string;
    effectiveAt: string;
    expiresAt: string;
    rows: Array<{
        targetKind: 'KubernetesTarget' | 'EdgeRuntimeTarget';
        providerId: 'runtime-target.lifecycle.kubernetes' | 'runtime-target.lifecycle.edge';
        observationSource: 'Agent' | 'CloudCore';
        actions: {
            create: 'REQUIRED' | 'UNSUPPORTED';
            import: 'REQUIRED' | 'UNSUPPORTED';
            upgrade: 'REQUIRED' | 'UNSUPPORTED';
            unmanage: 'REQUIRED' | 'UNSUPPORTED';
        };
    }>;
};
