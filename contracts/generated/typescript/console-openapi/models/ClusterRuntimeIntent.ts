/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { CreateKubernetesTargetIntent } from './CreateKubernetesTargetIntent';
import type { DeleteRuntimeTargetIntent } from './DeleteRuntimeTargetIntent';
import type { ImportEdgeRuntimeTargetIntent } from './ImportEdgeRuntimeTargetIntent';
import type { ImportKubernetesTargetIntent } from './ImportKubernetesTargetIntent';
import type { UpgradeRuntimeTargetIntent } from './UpgradeRuntimeTargetIntent';
export type ClusterRuntimeIntent = (CreateKubernetesTargetIntent | ImportKubernetesTargetIntent | ImportEdgeRuntimeTargetIntent | UpgradeRuntimeTargetIntent | DeleteRuntimeTargetIntent);
