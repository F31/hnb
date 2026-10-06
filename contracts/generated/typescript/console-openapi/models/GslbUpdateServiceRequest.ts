/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { GslbHealthCheck } from './GslbHealthCheck';
/**
 * GSLB 四期服务编辑输入：全部字段可选，仅提交需要变更的字段；
 * 合并后与现状一致时服务端不变更（幂等 200）。
 *
 */
export type GslbUpdateServiceRequest = {
    name?: string;
    requireApproval?: boolean;
    healthCheck?: GslbHealthCheck;
};
