/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { ClusterDetailResponse } from '../models/ClusterDetailResponse';
import type { ClusterListResponse } from '../models/ClusterListResponse';
import type { ClusterNodeListResponse } from '../models/ClusterNodeListResponse';
import type { ConnectivityState } from '../models/ConnectivityState';
import type { Freshness } from '../models/Freshness';
import type { HealthState } from '../models/HealthState';
import type { LifecycleState } from '../models/LifecycleState';
import type { ProblemDetails } from '../models/ProblemDetails';
import type { TargetKind } from '../models/TargetKind';
import type { CancelablePromise } from '../core/CancelablePromise';
import { OpenAPI } from '../core/OpenAPI';
import { request as __request } from '../core/request';
export class ClustersService {
    /**
     * List cluster targets with exact page totals
     * Returns only KubernetesTarget and EdgeRuntimeTarget rows ordered by updatedAt DESC, targetId DESC.
     * @returns ClusterListResponse Exact tenant-scoped cluster page.
     * @returns ProblemDetails RFC 9457 problem with bounded, non-sensitive extensions.
     * @throws ApiError
     */
    public static listClusters({
        xCorrelationId,
        page = 1,
        pageSize = 20,
        keyword,
        targetKind,
        lifecycleState,
        healthState,
        connectivityState,
        freshness,
    }: {
        xCorrelationId: string,
        page?: number,
        pageSize?: number,
        keyword?: string,
        targetKind?: TargetKind,
        lifecycleState?: LifecycleState,
        healthState?: HealthState,
        connectivityState?: ConnectivityState,
        freshness?: Freshness,
    }): CancelablePromise<ClusterListResponse | ProblemDetails> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/resources/clusters',
            headers: {
                'X-Correlation-ID': xCorrelationId,
            },
            query: {
                'page': page,
                'pageSize': pageSize,
                'keyword': keyword,
                'targetKind': targetKind,
                'lifecycleState': lifecycleState,
                'healthState': healthState,
                'connectivityState': connectivityState,
                'freshness': freshness,
            },
            errors: {
                400: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                403: `RFC 9457 problem with bounded, non-sensitive extensions.`,
            },
        });
    }
    /**
     * Get one cluster target
     * Inaccessible targets and RuntimeTarget kinds outside the cluster contract return a non-enumerating 404.
     * @returns ClusterDetailResponse Cluster target detail.
     * @returns ProblemDetails RFC 9457 problem with bounded, non-sensitive extensions.
     * @throws ApiError
     */
    public static getCluster({
        xCorrelationId,
        targetId,
    }: {
        xCorrelationId: string,
        targetId: string,
    }): CancelablePromise<ClusterDetailResponse | ProblemDetails> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/resources/clusters/{targetId}',
            path: {
                'targetId': targetId,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
            },
            errors: {
                404: `RFC 9457 problem with bounded, non-sensitive extensions.`,
            },
        });
    }
    /**
     * List projected cluster nodes with exact page totals
     * Reads the node projection only and never fans out to an Agent, CloudCore, or target.
     * @returns ClusterNodeListResponse Exact tenant-scoped node page.
     * @returns ProblemDetails RFC 9457 problem with bounded, non-sensitive extensions.
     * @throws ApiError
     */
    public static listClusterNodes({
        xCorrelationId,
        targetId,
        page = 1,
        pageSize = 50,
        keyword,
        lifecycleState,
        healthState,
        connectivityState,
        freshness,
    }: {
        xCorrelationId: string,
        targetId: string,
        page?: number,
        pageSize?: number,
        keyword?: string,
        lifecycleState?: LifecycleState,
        healthState?: HealthState,
        connectivityState?: ConnectivityState,
        freshness?: Freshness,
    }): CancelablePromise<ClusterNodeListResponse | ProblemDetails> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/resources/clusters/{targetId}/nodes',
            path: {
                'targetId': targetId,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
            },
            query: {
                'page': page,
                'pageSize': pageSize,
                'keyword': keyword,
                'lifecycleState': lifecycleState,
                'healthState': healthState,
                'connectivityState': connectivityState,
                'freshness': freshness,
            },
            errors: {
                400: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                404: `RFC 9457 problem with bounded, non-sensitive extensions.`,
            },
        });
    }
}
