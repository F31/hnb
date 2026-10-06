/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
/**
 * 服务级健康检查模板（迁移 094）；controller 探测成员集群时按服务读取，同集群多服务引用取最严格配置。
 */
export type GslbHealthCheck = {
    protocol: 'http' | 'https';
    path: string;
    intervalSeconds: number;
    timeoutSeconds: number;
    failureThreshold: number;
};
