/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { ApiVersion } from './ApiVersion';
import type { DictionaryEntry } from './DictionaryEntry';
export type DictionaryResponse = {
    apiVersion: ApiVersion;
    dictionaryId: string;
    version: string;
    compatibilityOnly?: boolean;
    entries: Array<DictionaryEntry>;
};
