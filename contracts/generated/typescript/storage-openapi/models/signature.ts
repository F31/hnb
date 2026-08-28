/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { digest } from './digest';
export type signature = {
    format: 'Cosign' | 'Notation' | 'X509';
    keyId: string;
    signedDigest: digest;
    evidenceRef: string;
};
