/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
/**
 * A menu entry. Parent groups carry children and may have an empty path;
 * leaf entries reference a route path.
 *
 */
export type NavigationItem = {
    title: string;
    path?: string;
    icon?: string;
    permission?: string;
    capability?: string;
    children?: Array<NavigationItem>;
};
