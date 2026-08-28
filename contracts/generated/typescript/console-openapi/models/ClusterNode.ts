/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { StateDimensions } from './StateDimensions';
export type ClusterNode = (StateDimensions & {
    nodeId: string;
    name: string;
    role?: string;
    architecture?: string;
    operatingSystem?: string;
    runtimeVersion?: string;
});
