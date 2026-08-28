/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { NavigationResponse } from '../models/NavigationResponse';
import type { ProblemDetails } from '../models/ProblemDetails';
import type { CancelablePromise } from '../core/CancelablePromise';
import { OpenAPI } from '../core/OpenAPI';
import { request as __request } from '../core/request';
export class NavigationService {
    /**
     * Get the tenant-scoped navigation response (menus and routes)
     * Server-driven navigation catalog (UI 规范 V2.6 §6). Combines user
     * identity, roles, tenant/space/locale, plugin enablement, License,
     * Feature Flag and Capability into a single NavigationResponse.
     * The ETag header enables conditional requests: a matching
     * If-None-Match returns 304 without a body.
     *
     * @returns NavigationResponse Tenant-scoped navigation catalog.
     * @returns ProblemDetails RFC 9457 problem with bounded, non-sensitive extensions.
     * @throws ApiError
     */
    public static getNavigationMenus({
        xCorrelationId,
        tenant,
        spaceId,
        locale = 'zh-CN',
        ifNoneMatch,
    }: {
        xCorrelationId: string,
        /**
         * Tenant id used to scope the navigation catalog.
         */
        tenant: string,
        /**
         * Optional workspace/space id further scoping the catalog.
         */
        spaceId?: string,
        /**
         * Locale used for localized menu titles. Defaults to zh-CN.
         */
        locale?: 'zh-CN' | 'en-US',
        /**
         * ETag from a previous response; 304 when current.
         */
        ifNoneMatch?: string,
    }): CancelablePromise<NavigationResponse | ProblemDetails> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/navigation/menus',
            headers: {
                'X-Correlation-ID': xCorrelationId,
                'If-None-Match': ifNoneMatch,
            },
            query: {
                'tenant': tenant,
                'spaceId': spaceId,
                'locale': locale,
            },
            errors: {
                304: `Cached navigation is current (If-None-Match matched).`,
                400: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                401: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                403: `RFC 9457 problem with bounded, non-sensitive extensions.`,
            },
        });
    }
}
