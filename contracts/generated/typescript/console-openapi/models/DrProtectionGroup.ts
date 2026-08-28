/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { DrLifecycleState } from './DrLifecycleState';
export type DrProtectionGroup = {
    id: string;
    tenantId: string;
    name: string;
    primaryRegion: string;
    standbyRegion: string;
    lifecycleState: DrLifecycleState;
    createdAt?: string;
    updatedAt?: string;
};
