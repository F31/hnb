/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { ApiVersion } from './ApiVersion';
import type { PageSchemaSpec } from './PageSchemaSpec';
import type { SchemaMetadata } from './SchemaMetadata';
export type PageSchema = {
    apiVersion: ApiVersion;
    kind: string;
    metadata: SchemaMetadata;
    spec: PageSchemaSpec;
};
