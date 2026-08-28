/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { secret_reference_schema } from './secret_reference_schema';
export type StorageDriverLifecycleIntent = {
    targetId: string;
    targetVersion: number;
    packageId: string;
    packageVersion: string;
    currentVersion?: string;
    kubernetesVersion: string;
    parameters?: Record<string, any>;
    secretReferences?: Array<secret_reference_schema>;
};
