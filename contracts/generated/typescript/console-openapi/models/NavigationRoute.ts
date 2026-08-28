/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
export type NavigationRoute = {
    name: string;
    path: string;
    pluginId: string;
    /**
     * Component key when the route is rendered by a plugin; schemaId routes omit it.
     */
    componentKey?: string;
    /**
     * PageSchema id when the route is rendered by SchemaPage (V2.6 §7).
     */
    schemaId?: string;
    redirect?: string;
    permission?: string;
    capability?: string;
    keepAlive?: boolean;
};
