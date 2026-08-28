/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { ClusterIntentKind } from './ClusterIntentKind';
import type { OperationProgress } from './OperationProgress';
import type { OperationStatus } from './OperationStatus';
import type { SafeFailure } from './SafeFailure';
import type { TargetKind } from './TargetKind';
export type OperationSummary = {
    operationId: string;
    intentId: string;
    type: ClusterIntentKind;
    status: OperationStatus;
    targetId: string;
    targetKind: TargetKind;
    progress: OperationProgress;
    failure?: SafeFailure;
    correlationId: string;
    createdAt: string;
    updatedAt: string;
    completedAt?: string;
};
