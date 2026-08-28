/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { CapabilitySnapshotSummary } from './CapabilitySnapshotSummary';
import type { StateDimensions } from './StateDimensions';
import type { TargetKind } from './TargetKind';
export type ClusterSummary = (StateDimensions & {
    targetId: string;
    targetKind: TargetKind;
    displayName: string;
    source: 'created' | 'imported';
    staleThresholdSeconds: number;
    runtimeVersion?: string;
    nodeCount: number;
    capabilitySnapshot?: CapabilitySnapshotSummary;
    createdAt: string;
    updatedAt: string;
});
