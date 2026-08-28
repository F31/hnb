/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { NavigationContext } from './NavigationContext';
import type { NavigationMenu } from './NavigationMenu';
import type { NavigationPlugin } from './NavigationPlugin';
import type { NavigationRoute } from './NavigationRoute';
export type NavigationResponse = {
    apiVersion: string;
    /**
     * Version tag echoed in the ETag header for conditional requests.
     */
    etag: string;
    generatedAt: string;
    context: NavigationContext;
    /**
     * Content versions (permission / pluginCatalog / navigation / license).
     */
    versions: Record<string, string>;
    plugins?: Array<NavigationPlugin>;
    menus: Array<NavigationMenu>;
    routes: Array<NavigationRoute>;
};
