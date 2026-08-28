/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { DictionaryResponse } from '../models/DictionaryResponse';
import type { ProblemDetails } from '../models/ProblemDetails';
import type { CancelablePromise } from '../core/CancelablePromise';
import { OpenAPI } from '../core/OpenAPI';
import { request as __request } from '../core/request';
export class DictionariesService {
    /**
     * Get a server-owned cluster state dictionary
     * @returns DictionaryResponse Versioned dictionary labels and semantic tokens.
     * @returns ProblemDetails RFC 9457 problem with bounded, non-sensitive extensions.
     * @throws ApiError
     */
    public static getClusterDictionary({
        xCorrelationId,
        dictionaryId,
    }: {
        xCorrelationId: string,
        dictionaryId: 'resource.cluster.lifecycle' | 'resource.cluster.health' | 'resource.cluster.connectivity' | 'resource.cluster.freshness' | 'resource.cluster.status',
    }): CancelablePromise<DictionaryResponse | ProblemDetails> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/dictionaries/{dictionaryId}',
            path: {
                'dictionaryId': dictionaryId,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
            },
            errors: {
                404: `RFC 9457 problem with bounded, non-sensitive extensions.`,
            },
        });
    }
}
