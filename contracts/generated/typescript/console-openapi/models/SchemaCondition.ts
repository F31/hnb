/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { SchemaConditionTerm } from './SchemaConditionTerm';
/**
 * Controlled condition DSL (V2.6 §4.3). Only permission / feature /
 * license / capability / context terms are allowed.
 *
 */
export type SchemaCondition = {
    all?: Array<SchemaConditionTerm>;
    any?: Array<SchemaConditionTerm>;
};
