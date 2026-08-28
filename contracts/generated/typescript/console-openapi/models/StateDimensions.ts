/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { ConnectivityState } from './ConnectivityState';
import type { Freshness } from './Freshness';
import type { HealthState } from './HealthState';
import type { LifecycleState } from './LifecycleState';
export type StateDimensions = {
    lifecycleState: LifecycleState;
    healthState: HealthState;
    connectivityState: ConnectivityState;
    freshness: Freshness;
    observedAt?: string;
    lastKnownStateAt: string;
};
