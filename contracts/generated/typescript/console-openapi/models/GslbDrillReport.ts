/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { GslbDrillReportPayload } from './GslbDrillReportPayload';
import type { GslbDrillVerdict } from './GslbDrillVerdict';
export type GslbDrillReport = {
    id: string;
    tenantId: string;
    serviceId: string;
    requestId: string;
    verdict: GslbDrillVerdict;
    report: GslbDrillReportPayload;
    createdAt: string;
};
