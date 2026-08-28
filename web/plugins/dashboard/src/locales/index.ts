import { definePluginMessages } from '@hnb/plugin-sdk'

export default definePluginMessages({
  'zh-CN': {
    menu: {
      overview: '平台总览',
      approvals: '审批待办',
      recent: '最近操作',
    },
    page: {
      title: '平台运行总览',
    },
    clusterCount: '集群数量',
    cpu: 'CPU',
    gpu: 'GPU',
    storage: '存储',
    cores: '核',
    cards: '卡',
    common: {
      loading: '加载中...',
    },
    approvals: {
      title: '审批待办',
      desc: '待审批的操作列表',
      empty: '暂无待审批操作',
      pending: '有 {count} 个操作待审批',
      viewAll: '查看全部',
    },
    recent: {
      title: '最近操作',
      desc: '最近操作历史列表（待对接 API）',
    },
  },
  'en-US': {
    menu: {
      overview: 'Overview',
      approvals: 'Approvals',
      recent: 'Recent Operations',
    },
    page: {
      title: 'Platform Overview',
    },
    clusterCount: 'Clusters',
    cpu: 'CPU',
    gpu: 'GPU',
    storage: 'Storage',
    cores: 'Cores',
    cards: 'Cards',
    common: {
      loading: 'Loading...',
    },
    approvals: {
      title: 'Pending Approvals',
      desc: 'Operations awaiting approval',
      empty: 'No pending approvals',
      pending: '{count} operation(s) pending approval',
      viewAll: 'View All',
    },
    recent: {
      title: 'Recent Operations',
      desc: 'Recent operation history (API integration pending)',
    },
  },
})
