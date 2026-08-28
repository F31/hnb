/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { ApiVersion } from './ApiVersion';
import type { OperationAction } from './OperationAction';
import type { OperationStatus } from './OperationStatus';
export type OperationActionsResponse = {
    apiVersion: ApiVersion;
    operationId: string;
    status: OperationStatus;
    allowedActions: Array<OperationAction>;
    version: number;
};
