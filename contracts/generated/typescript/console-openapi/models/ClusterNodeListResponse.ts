/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { ApiVersion } from './ApiVersion';
import type { ClusterNode } from './ClusterNode';
import type { PageMetadata } from './PageMetadata';
import type { TargetKind } from './TargetKind';
export type ClusterNodeListResponse = {
    apiVersion: ApiVersion;
    targetId: string;
    targetKind: TargetKind;
    items: Array<ClusterNode>;
    pagination: PageMetadata;
};
