/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { secret_reference_schema } from './secret_reference_schema';
export type kubernetes_lifecycle_step_input_schema = {
    schemaVersion: string;
    targetId: string;
    targetKind: string;
    action: 'create' | 'import' | 'upgrade' | 'unmanage' | 'node-add';
    displayName?: string;
    desiredVersion?: string;
    credentialSecretRef?: secret_reference_schema;
    sshSecretRef?: secret_reference_schema;
    parameters?: Record<string, any>;
    idempotencyKey: string;
    fencingGeneration: number;
    observationVersion: number;
};
