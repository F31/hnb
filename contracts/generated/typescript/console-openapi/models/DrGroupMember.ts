/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { DrMemberType } from './DrMemberType';
export type DrGroupMember = {
    id: string;
    groupId: string;
    memberType: DrMemberType;
    /**
     * gslb_service 成员为 GSLB 服务 id；data_layer_ref 为数据层引用标识。
     */
    refId: string;
    name: string;
    createdAt?: string;
};
