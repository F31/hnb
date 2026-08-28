/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { GslbPoolMember } from './GslbPoolMember';
export type GslbPool = {
    id: string;
    serviceId: string;
    name: string;
    priority: number;
    members?: Array<GslbPoolMember>;
};
