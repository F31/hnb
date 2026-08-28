/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { DrGroupMember } from './DrGroupMember';
import type { DrProtectionGroup } from './DrProtectionGroup';
import type { DrSwitchRun } from './DrSwitchRun';
export type DrGroupDetailResponse = {
    group: DrProtectionGroup;
    members: Array<DrGroupMember>;
    runs: Array<DrSwitchRun>;
};
