/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { runtime_intent_schema } from './runtime_intent_schema';
export type RuntimeIntentRecord = {
    intentId: string;
    status: 'received' | 'validated' | 'planned' | 'operationCommitted' | 'rejected';
    semanticDigest: string;
    intent: runtime_intent_schema;
    executionPlanId?: string;
    operationId?: string;
    createdAt: string;
};
