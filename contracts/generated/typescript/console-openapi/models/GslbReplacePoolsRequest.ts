/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { GslbPoolInput } from './GslbPoolInput';
/**
 * GSLB 四期地址池整组替换输入：池以 name 为身份（服务端重新生成 pool id），
 * 当前 active 池的名字必须仍在新配置中，否则 409（先 failover 到保留的池）。
 *
 */
export type GslbReplacePoolsRequest = {
    pools: Array<GslbPoolInput>;
};
