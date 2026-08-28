/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type provider_backend_schema_schema = {
    schemaVersion: string;
    providerType: string;
    providerSchemaVersion: string;
    componentType: string;
    fields: Array<{
        name: string;
        label: string;
        type: 'text' | 'number' | 'boolean' | 'select';
        required: boolean;
        options?: Array<string>;
    }>;
};
