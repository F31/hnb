/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type SchemaDataSourceDefinition = {
    id: string;
    type: 'query' | 'paginatedQuery' | 'aggregate' | 'dictionary' | 'stream' | 'operationStatus';
    endpointId: string;
    method?: 'GET' | 'POST';
    contextBindings?: Array<string>;
    queryBindings?: Array<string>;
    responseMapping?: Record<string, string>;
    /**
     * Client-side response cache configuration (V2.6 §13.6).
     */
    cache?: Record<string, any>;
};
