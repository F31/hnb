/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { secret_reference_schema } from './secret_reference_schema';
export type StorageBackendInput = {
    providerType: string;
    providerSchemaVersion: string;
    backendId: string;
    displayName: string;
    description?: string;
    secretReference?: secret_reference_schema;
    attributes?: Record<string, (string | number | boolean)>;
};
