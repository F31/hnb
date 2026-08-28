/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { SchemaCondition } from './SchemaCondition';
export type SchemaRegion = {
    id: string;
    /**
     * Trusted component type registered in the ComponentRegistry (V2.6 §8).
     */
    componentType: string;
    /**
     * Declarative extension point namespace; takes precedence over componentType when set.
     */
    extensionPoint?: string;
    span?: number;
    responsive?: Record<string, number>;
    props?: Record<string, any>;
    condition?: SchemaCondition;
};
