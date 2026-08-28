/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type RiskConfirmation = {
    acknowledged: boolean;
    /**
     * Opaque, short-lived challenge proof; never interpreted as authorization by the caller.
     */
    confirmation: string;
    reason?: string;
};
