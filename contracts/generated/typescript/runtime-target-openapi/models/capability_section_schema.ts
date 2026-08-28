/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type capability_section_schema = {
    snapshotId: string;
    digest: string;
    kubernetesVersion?: string;
    kubeEdgeVersion?: string;
    runtimeVersion: string;
    architectures: Array<string>;
    resources: {
        cpuMillis: number;
        memoryBytes: number;
        gpuCount?: number;
        npuCount?: number;
    };
    cniPlugins?: Array<string>;
    csiDrivers?: Array<string>;
    gatewayApiVersions?: Array<string>;
    securityFeatures?: Array<string>;
    storageClasses?: Array<string>;
};
