/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { digest } from './digest';
import type { semanticVersion } from './semanticVersion';
export type conformanceEvidence = {
    packageVersion: semanticVersion;
    kubernetesVersion: semanticVersion;
    suiteVersion: semanticVersion;
    passedAt: string;
    expiresAt: string;
    evidenceRef: string;
    evidenceDigest: digest;
};
