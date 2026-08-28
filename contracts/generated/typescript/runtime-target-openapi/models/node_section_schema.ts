/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type node_section_schema = {
    nodeId: string;
    name?: string;
    lifecycleState: 'UNKNOWN' | 'REGISTERING' | 'PROVISIONING' | 'ACTIVE' | 'UPGRADING' | 'FAILED' | 'DELETING' | 'TERMINATED';
    healthState: 'UNKNOWN' | 'HEALTHY' | 'DEGRADED' | 'UNHEALTHY';
    connectivityState: 'UNKNOWN' | 'CONNECTED' | 'DISCONNECTED';
    freshness: 'UNKNOWN' | 'FRESH' | 'STALE';
    observedAt: string;
    lastKnownStateAt: string;
    deleted?: boolean;
    runtimeVersion?: string;
    kubeletVersion?: string;
    architecture?: string;
    resources: {
        cpuMillis: number;
        memoryBytes: number;
        gpuCount?: number;
        npuCount?: number;
    };
    labels?: Record<string, string>;
};
