/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { capability_section_schema } from './capability_section_schema';
import type { node_section_schema } from './node_section_schema';
import type { storage_inventory_section_schema } from './storage_inventory_section_schema';
export type runtime_target_observation_schema = {
    schemaVersion: string;
    eventId: string;
    tenantId: string;
    targetId: string;
    targetKind: 'KubernetesTarget' | 'EdgeRuntimeTarget';
    observerId: string;
    observerKind: 'Agent' | 'CloudCore';
    observerGeneration: number;
    sequence: number;
    observedAt: string;
    inventoryMode: 'Full' | 'Delta';
    target?: {
        lifecycleState: 'UNKNOWN' | 'REGISTERING' | 'PROVISIONING' | 'ACTIVE' | 'UPGRADING' | 'FAILED' | 'DELETING' | 'TERMINATED';
        healthState: 'UNKNOWN' | 'HEALTHY' | 'DEGRADED' | 'UNHEALTHY';
        connectivityState: 'UNKNOWN' | 'CONNECTED' | 'DISCONNECTED';
        lastKnownStateAt: string;
        staleThresholdSeconds: number;
        runtimeVersion?: string;
    };
    capability?: capability_section_schema;
    nodes?: Array<node_section_schema>;
    storageInventory?: storage_inventory_section_schema;
};
