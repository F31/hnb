/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type SchemaEndpointDefinition = {
    id: string;
    /**
     * Trusted relative path; only allowlisted prefixes are accepted by the client.
     */
    path: string;
    method?: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';
};
