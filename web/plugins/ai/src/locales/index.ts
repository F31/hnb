import { definePluginMessages } from '@hnb/plugin-sdk'

export default definePluginMessages({
  'zh-CN': {
    menu: { models: '模型仓库', inference: '推理服务', gateway: 'AI网关' },
    models: { title: '模型仓库', desc: '模型上传/下载/版本管理（待对接 API）' },
    inference: { title: '推理服务', desc: '推理部署与弹性伸缩（待对接 API）' },
    gateway: { title: 'AI网关', desc: 'AI 模型路由与限流鉴权（待对接 API）' },
  },
  'en-US': {
    menu: { models: 'Model Registry', inference: 'Inference', gateway: 'AI Gateway' },
    models: { title: 'Model Registry', desc: 'Model upload/download and version management (API integration pending)' },
    inference: { title: 'Inference Service', desc: 'Inference deployment and autoscaling (API integration pending)' },
    gateway: { title: 'AI Gateway', desc: 'AI model routing, rate limiting and auth (API integration pending)' },
  },
})
