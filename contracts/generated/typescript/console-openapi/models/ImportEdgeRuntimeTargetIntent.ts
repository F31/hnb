/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { IntentMetadata } from './IntentMetadata';
import type { secret_reference_schema } from './secret_reference_schema';
export type ImportEdgeRuntimeTargetIntent = {
    apiVersion: string;
    kind: string;
    metadata: IntentMetadata;
    spec: {
        targetKind: string;
        displayName: string;
        /**
         * CloudCore target address only; userinfo, query, and fragment are forbidden.
         */
        cloudCoreEndpoint: string;
        credentialSecretRef: secret_reference_schema;
        nodeGroupMappings?: Record<string, string>;
    };
};
