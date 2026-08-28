/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { SchemaAction } from './SchemaAction';
import type { SchemaDataSourceDefinition } from './SchemaDataSourceDefinition';
import type { SchemaEndpointDefinition } from './SchemaEndpointDefinition';
import type { SchemaRegion } from './SchemaRegion';
export type PageSchemaSpec = {
    template: 'list' | 'detail' | 'form' | 'dashboard' | 'wizard' | 'split' | 'settings' | 'custom';
    titleKey?: string;
    descriptionKey?: string;
    contextRequirements?: Array<string>;
    layout?: Record<string, any>;
    endpoints?: Array<SchemaEndpointDefinition>;
    dataSources?: Array<SchemaDataSourceDefinition>;
    actions?: Array<SchemaAction>;
    regions: Array<SchemaRegion>;
};
