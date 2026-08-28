/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type SchemaActionRequest = {
    method: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';
    /**
     * Trusted endpoint identifier registered with the DataSourceManager allowlist.
     */
    endpointId: string;
    pathParams?: Array<string>;
    queryParams?: Array<string>;
};
