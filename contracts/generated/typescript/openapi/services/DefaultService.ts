/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { ContractEchoRequest } from '../models/ContractEchoRequest';
import type { ContractEchoResponse } from '../models/ContractEchoResponse';
import type { ProblemDetails } from '../models/ProblemDetails';
import type { CancelablePromise } from '../core/CancelablePromise';
import { OpenAPI } from '../core/OpenAPI';
import { request as __request } from '../core/request';
export class DefaultService {
    /**
     * Verify generated clients and common transport types
     * @returns ContractEchoResponse Contract data round-tripped without a runtime mutation.
     * @returns ProblemDetails RFC 9457 problem response.
     * @throws ApiError
     */
    public static echoContract({
        xCorrelationId,
        idempotencyKey,
        requestBody,
    }: {
        xCorrelationId: string,
        idempotencyKey: string,
        requestBody: ContractEchoRequest,
    }): CancelablePromise<ContractEchoResponse | ProblemDetails> {
        return __request(OpenAPI, {
            method: 'POST',
            url: '/v1/contract-echo',
            headers: {
                'X-Correlation-ID': xCorrelationId,
                'Idempotency-Key': idempotencyKey,
            },
            body: requestBody,
            mediaType: 'application/json',
            errors: {
                400: `The contract fixture is invalid.`,
            },
        });
    }
}
