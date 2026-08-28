/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { PluginDependencies } from './PluginDependencies';
import type { PluginMenu } from './PluginMenu';
import type { PluginPermissionSet } from './PluginPermissionSet';
export type NavigationPlugin = {
    id?: string;
    name: string;
    version: string;
    displayName: string;
    tier: 'T0' | 'T1' | 'T2' | 'T3';
    enabled: boolean;
    mode: 'local' | 'remote';
    permissions: PluginPermissionSet;
    capabilities: PluginPermissionSet;
    dependencies: PluginDependencies;
    menu: PluginMenu;
};
