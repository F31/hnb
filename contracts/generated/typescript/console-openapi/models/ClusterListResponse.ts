/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { ApiVersion } from './ApiVersion';
import type { ClusterSummary } from './ClusterSummary';
import type { PageMetadata } from './PageMetadata';
export type ClusterListResponse = {
    apiVersion: ApiVersion;
    items: Array<ClusterSummary>;
    pagination: PageMetadata;
};
