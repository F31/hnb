/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { OperationAction } from './OperationAction';
import type { OperationLinks } from './OperationLinks';
import type { OperationStep } from './OperationStep';
import type { OperationSummary } from './OperationSummary';
export type OperationDetail = (OperationSummary & {
    executionPlanId: string;
    steps: Array<OperationStep>;
    allowedActions: Array<OperationAction>;
    links: OperationLinks;
});
