/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { secret_reference_schema } from './secret_reference_schema';
/**
 * Immutable, server-authored execution snapshot. Published instances must never be mutated.
 */
export type execution_plan_schema = {
    planId: string;
    intentId: string;
    semanticDigest: string;
    releaseRef: string;
    artifactDigests: Array<string>;
    targetRef: string;
    capabilitySnapshotDigest: string;
    providerVersions: Record<string, string>;
    policyDecisionRefs: Array<string>;
    approvedParameters: Record<string, (string | number | boolean | null)>;
    secretReferences: Array<secret_reference_schema>;
    compatibilityDecision?: {
        matrixVersion: string;
        providerProtocolVersion: string;
        targetKind: 'KubernetesTarget' | 'EdgeRuntimeTarget';
        action: 'create' | 'import' | 'upgrade' | 'unmanage';
        status: string;
        providerId: string;
        observationSource: 'Agent' | 'CloudCore';
        effectiveAt: string;
        expiresAt: string;
    };
    targetSnapshot?: {
        targetId: string;
        targetKind: 'KubernetesTarget' | 'EdgeRuntimeTarget';
        projectionVersion: number;
        observationSource: 'Agent' | 'CloudCore';
    };
    steps: Array<{
        stepId: string;
        stepType: string;
        providerId?: string;
        providerVersion?: string;
        providerDigest?: string;
        providerProtocolVersion?: string;
        dependsOn: Array<string>;
        targetRef?: string;
        targetKind?: 'KubernetesTarget' | 'EdgeRuntimeTarget';
        inputSchema?: string;
        inputs?: Record<string, any>;
        secretReferences?: Array<secret_reference_schema>;
        idempotencyKey?: string;
        fencingPolicy?: 'monotonic-worker-lease-v2' | 'target-projection-and-storageclass-resource-version';
        retryPolicy?: {
            maxAttempts: number;
            backoff: 'none' | 'exponential';
        };
        timeoutSeconds?: number;
        compensation?: {
            type: 'none' | 'unregister' | 'rollback';
            ownershipScope: 'none' | 'operation-owned-only' | 'management-relation-only';
        };
    }>;
    createdAt: string;
};
