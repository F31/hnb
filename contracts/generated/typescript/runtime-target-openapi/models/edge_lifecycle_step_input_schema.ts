/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { secret_reference_schema } from './secret_reference_schema';
export type edge_lifecycle_step_input_schema = {
    schemaVersion: string;
    targetId: string;
    targetKind: string;
    action: 'import' | 'upgrade' | 'unmanage';
    displayName?: string;
    /**
     * Normalized CloudCore target address; userinfo, query, and fragment are forbidden.
     */
    cloudCoreEndpoint?: string;
    credentialSecretRef?: secret_reference_schema;
    nodeGroupMappings?: Record<string, string>;
    desiredVersion?: string;
    idempotencyKey: string;
    fencingGeneration: number;
    observationVersion: number;
};
