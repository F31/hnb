/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { IntentMetadata } from './IntentMetadata';
import type { RiskConfirmation } from './RiskConfirmation';
import type { TargetKind } from './TargetKind';
export type DeleteRuntimeTargetIntent = {
    apiVersion: string;
    kind: string;
    metadata: IntentMetadata;
    spec: {
        targetId: string;
        targetKind: TargetKind;
        expectedVersion: number;
        riskConfirmation?: RiskConfirmation;
    };
};
