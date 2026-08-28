/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
/**
 * Request metadata only. On protected routes tenantId and actorId are non-authoritative inputs and are overwritten by the verified trusted context tenantId/membershipId claims.
 */
export type RequestContext = {
    tenantId: string;
    projectId?: string;
    environmentId?: string;
    actorId: string;
    correlationId: string;
    traceparent?: string;
};
