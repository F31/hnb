/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { capabilities } from './capabilities';
import type { compatibility } from './compatibility';
import type { conformanceEvidence } from './conformanceEvidence';
import type { digest } from './digest';
import type { requiredComponents } from './requiredComponents';
import type { semanticVersion } from './semanticVersion';
import type { signature } from './signature';
export type storage_driver_package_schema = {
    schemaVersion: string;
    packageId: string;
    packageVersion: semanticVersion;
    provisioners: Array<string>;
    compatibility: compatibility;
    capabilities: capabilities;
    requiredComponents: requiredComponents;
    packageDigest: digest;
    signature: signature;
    conformanceEvidence: Array<conformanceEvidence>;
};
