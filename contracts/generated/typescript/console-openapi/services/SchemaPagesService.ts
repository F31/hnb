/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { PageSchema } from '../models/PageSchema';
import type { ProblemDetails } from '../models/ProblemDetails';
import type { RollbackRequest } from '../models/RollbackRequest';
import type { CancelablePromise } from '../core/CancelablePromise';
import { OpenAPI } from '../core/OpenAPI';
import { request as __request } from '../core/request';
export class SchemaPagesService {
    /**
     * Get one declarative PageSchema by page id
     * Returns the declarative PageSchema for a standard page (UI 规范 V2.6 §7).
     * The payload only carries trusted componentType / endpointId / actionId
     * identifiers and never executable code (V2.6 §2.2 / §3.4).
     *
     * @returns PageSchema PageSchema envelope with revision metadata.
     * @returns ProblemDetails RFC 9457 problem with bounded, non-sensitive extensions.
     * @throws ApiError
     */
    public static getPageSchema({
        xCorrelationId,
        id,
    }: {
        xCorrelationId: string,
        /**
         * PageSchema id, e.g. `cluster.list` (V2.6 §22.2 naming).
         */
        id: string,
    }): CancelablePromise<PageSchema | ProblemDetails> {
        return __request(OpenAPI, {
            method: 'GET',
            url: '/api/v1/schema/page/{id}',
            path: {
                'id': id,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
            },
            errors: {
                400: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                401: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                403: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                404: `RFC 9457 problem with bounded, non-sensitive extensions.`,
            },
        });
    }
    /**
     * Publish a PageSchema as the next revision
     * Publishes a declarative PageSchema as a new revision (UI 规范 V2.6 §20.3):
     * bumps the revision, appends to the immutable history and enqueues an
     * outbox event (hnb.event.ui.page-published.v1) in the same transaction.
     * Requires schema:update. Admin write operation.
     *
     * @returns PageSchema The published PageSchema with its new revision and ETag.
     * @returns ProblemDetails RFC 9457 problem with bounded, non-sensitive extensions.
     * @throws ApiError
     */
    public static publishPageSchema({
        xCorrelationId,
        idempotencyKey,
        id,
        requestBody,
    }: {
        xCorrelationId: string,
        idempotencyKey: string,
        /**
         * PageSchema id, e.g. `cluster.list` (V2.6 §22.2 naming).
         */
        id: string,
        requestBody: PageSchema,
    }): CancelablePromise<PageSchema | ProblemDetails> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/ui/pages/{id}/publish',
            path: {
                'id': id,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
                'Idempotency-Key': idempotencyKey,
            },
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                400: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                401: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                403: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                404: `RFC 9457 problem with bounded, non-sensitive extensions.`,
            },
        });
    }
    /**
     * Roll the active PageSchema back to a historical revision
     * Switches the active revision without overwriting history (UI 规范 V2.6 §20.4)
     * and enqueues an outbox event (hnb.event.ui.page-rolled-back.v1) in the
     * same transaction. Requires schema:update. Admin write operation.
     *
     * @returns PageSchema The PageSchema now active after the rollback.
     * @returns ProblemDetails RFC 9457 problem with bounded, non-sensitive extensions.
     * @throws ApiError
     */
    public static rollbackPageSchema({
        xCorrelationId,
        idempotencyKey,
        id,
        requestBody,
    }: {
        xCorrelationId: string,
        idempotencyKey: string,
        /**
         * PageSchema id, e.g. `cluster.list` (V2.6 §22.2 naming).
         */
        id: string,
        requestBody: RollbackRequest,
    }): CancelablePromise<PageSchema | ProblemDetails> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/api/v1/ui/pages/{id}/rollback',
            path: {
                'id': id,
            },
            headers: {
                'X-Correlation-ID': xCorrelationId,
                'Idempotency-Key': idempotencyKey,
            },
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                400: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                401: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                403: `RFC 9457 problem with bounded, non-sensitive extensions.`,
                404: `RFC 9457 problem with bounded, non-sensitive extensions.`,
            },
        });
    }
}
