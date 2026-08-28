/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { resource } from './resource';
export type RetainedVolumeIntent = {
    targetId: string;
    targetVersion: number;
    workflowProviderRef: string;
    persistentVolume: resource;
    persistentVolumeClaim: Record<string, any>;
    podDependencies: any[];
    statefulSetDependencies: any[];
    approvalAcknowledged: boolean;
};
