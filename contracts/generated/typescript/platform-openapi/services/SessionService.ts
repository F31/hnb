/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { console_bootstrap_schema } from '../models/console_bootstrap_schema';
import type { problem_details_schema } from '../models/problem_details_schema';
import type { tenant_switch_schema } from '../models/tenant_switch_schema';
import type { CancelablePromise } from '../core/CancelablePromise';
import { OpenAPI } from '../core/OpenAPI';
import { request as __request } from '../core/request';
export class SessionService {
    /**
     * Get the authoritative Console access bootstrap
     * @returns console_bootstrap_schema Verified subject, active tenant, capabilities, and permissions.
     * @returns problem_details_schema RFC 9457 problem response.
     * @throws ApiError
     */
    public static getSessionBootstrap({
        xCorrelationId,
    }: {
        xCorrelationId: string,
    }): CancelablePromise<console_bootstrap_schema | problem_details_schema> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/v1/session/bootstrap',
            headers: {
                'X-Correlation-ID': xCorrelationId,
            },
            errors: {
                400: `RFC 9457 problem response.`,
            },
        });
    }
    /**
     * Select an authorized tenant and return a fresh bootstrap
     * @returns console_bootstrap_schema Tenant selection succeeded and all scoped access data was refreshed.
     * @returns problem_details_schema RFC 9457 problem response.
     * @throws ApiError
     */
    public static switchSessionTenant({
        xCorrelationId,
        idempotencyKey,
        ifMatch,
        requestBody,
    }: {
        xCorrelationId: string,
        idempotencyKey: string,
        ifMatch: string,
        requestBody: tenant_switch_schema,
    }): CancelablePromise<console_bootstrap_schema | problem_details_schema> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/v1/session/tenant-switch',
            headers: {
                'X-Correlation-ID': xCorrelationId,
                'Idempotency-Key': idempotencyKey,
                'If-Match': ifMatch,
            },
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                400: `RFC 9457 problem response.`,
            },
        });
    }
}
