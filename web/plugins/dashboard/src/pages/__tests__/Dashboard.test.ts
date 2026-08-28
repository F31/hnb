import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import Dashboard from '../Dashboard.vue'
import messages from '../../locales'

const i18n = createI18n({
  legacy: false,
  locale: 'zh-CN',
  fallbackLocale: 'zh-CN',
  messages: {
    'zh-CN': { dashboard: messages['zh-CN']! },
    'en-US': { dashboard: messages['en-US']! },
  },
})

function mountDashboard() {
  return mount(Dashboard, { global: { plugins: [i18n] } })
}

describe('Dashboard (schema-driven)', () => {
  it('renders the page title from i18n messages', () => {
    const wrapper = mountDashboard()
    expect(wrapper.find('.page-title').text()).toBe('平台运行总览')
  })

  it('renders all four metric cards', () => {
    const wrapper = mountDashboard()
    const cards = wrapper.findAll('.metric-card')
    expect(cards).toHaveLength(4)
    const text = wrapper.text()
    expect(text).toContain('集群数量')
    expect(text).toContain('CPU')
    expect(text).toContain('GPU')
    expect(text).toContain('存储')
  })

  it('renders the alerts region as a description list', () => {
    const wrapper = mountDashboard()
    expect(wrapper.find('.description-list').exists()).toBe(true)
    expect(wrapper.text()).toContain('节点资源不足')
  })

  it('has no schema validation or region errors', () => {
    const wrapper = mountDashboard()
    expect(wrapper.find('.page-error').exists()).toBe(false)
    expect(wrapper.findAll('.region-placeholder')).toHaveLength(0)
  })
})
