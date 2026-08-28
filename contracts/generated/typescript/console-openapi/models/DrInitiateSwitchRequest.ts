/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { DrSwitchDirection } from './DrSwitchDirection';
export type DrInitiateSwitchRequest = {
    direction: DrSwitchDirection;
    reason?: string;
    idempotencyKey: string;
};
