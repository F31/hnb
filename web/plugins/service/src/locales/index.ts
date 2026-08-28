import { definePluginMessages } from '@hnb/plugin-sdk'

export default definePluginMessages({
  'zh-CN': {
    menu: { data: '数据服务', messaging: '消息服务', governance: '微服务治理', gateway: '网关服务' },
    data: { title: '数据服务', desc: 'MySQL / PostgreSQL / Redis 实例管理（待对接 API）' },
    messaging: { title: '消息服务', desc: 'Kafka / RabbitMQ 实例管理（待对接 API）' },
    governance: { title: '微服务治理', desc: '服务列表、健康检查、流量管理（待对接 API）' },
    gateway: { title: '网关服务', desc: 'API Gateway 路由与插件管理（待对接 API）' },
  },
  'en-US': {
    menu: { data: 'Data Service', messaging: 'Message Service', governance: 'Microservice Governance', gateway: 'Gateway Service' },
    data: { title: 'Data Service', desc: 'MySQL / PostgreSQL / Redis instance management (API integration pending)' },
    messaging: { title: 'Message Service', desc: 'Kafka / RabbitMQ instance management (API integration pending)' },
    governance: { title: 'Microservice Governance', desc: 'Service list, health checks and traffic management (API integration pending)' },
    gateway: { title: 'Gateway Service', desc: 'API Gateway route and plugin management (API integration pending)' },
  },
})
