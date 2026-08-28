/**
 * 平台总览页 Schema（Schema 驱动试点，V2.5 §7/§11）。
 *
 * 当前为插件内置静态 Schema：结构先行，后续由服务端
 * 下发同构 Schema 时渲染层无需改动。
 */

import type { PageSchema } from '@hnb/schema-engine'

export const dashboardSchema: PageSchema = {
  apiVersion: 'ui.hnb.io/v1',
  kind: 'PageSchema',
  metadata: { id: 'dashboard.overview', revision: 1 },
  spec: {
    template: 'dashboard',
    titleKey: 'dashboard.title',
    layout: { type: 'grid' },
    regions: [
      {
        id: 'metric-clusters',
        componentType: 'MetricCard',
        span: 3,
        props: { title: '集群数量', value: 12, description: '生产 8 · 测试 4' },
      },
      {
        id: 'metric-cpu',
        componentType: 'MetricCard',
        span: 3,
        props: { title: 'CPU', value: 600, unit: '核', description: '使用率 39%' },
      },
      {
        id: 'metric-gpu',
        componentType: 'MetricCard',
        span: 3,
        props: { title: 'GPU', value: 16, unit: '卡', description: '显存使用 45%' },
      },
      {
        id: 'metric-storage',
        componentType: 'MetricCard',
        span: 3,
        props: { title: '存储', value: '7.1TB', description: '已使用 1.5TB' },
      },
      {
        id: 'alerts',
        componentType: 'DescriptionList',
        span: 12,
        props: {
          column: 1,
          items: [
            { label: '严重', value: 'Running 状态 Pod 数量超过阈值' },
            { label: '警告', value: '节点资源不足' },
            { label: '错误', value: '应用发布失败' },
          ],
        },
      },
    ],
  },
}
