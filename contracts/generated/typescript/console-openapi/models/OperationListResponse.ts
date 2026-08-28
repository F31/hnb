/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { ApiVersion } from './ApiVersion';
import type { OperationSummary } from './OperationSummary';
import type { PageMetadata } from './PageMetadata';
export type OperationListResponse = {
    apiVersion: ApiVersion;
    items: Array<OperationSummary>;
    pagination: PageMetadata;
};
