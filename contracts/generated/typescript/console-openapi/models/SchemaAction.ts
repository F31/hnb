/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { SchemaActionRequest } from './SchemaActionRequest';
import type { SchemaActionRoute } from './SchemaActionRoute';
import type { SchemaCondition } from './SchemaCondition';
export type SchemaAction = {
    id: string;
    type: 'navigate' | 'api' | 'operation' | 'workflow' | 'download' | 'openDrawer' | 'openModal' | 'emitEvent';
    labelKey?: string;
    permission?: string;
    enabledWhen?: SchemaCondition;
    confirm?: Record<string, any>;
    request?: SchemaActionRequest;
    route?: SchemaActionRoute;
    event?: Record<string, any>;
    result?: Record<string, any>;
};
