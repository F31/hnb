/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { SafeFailure } from './SafeFailure';
export type OperationStep = {
    stepId: string;
    name: string;
    status: 'pending' | 'queued' | 'in_progress' | 'paused' | 'succeeded' | 'failed' | 'cancelled' | 'compensating';
    attempt: number;
    startedAt?: string;
    completedAt?: string;
    failure?: SafeFailure;
};
