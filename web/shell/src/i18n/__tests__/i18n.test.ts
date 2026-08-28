import { describe, it, expect } from 'vitest'
import { i18n, setLocale, getLocale, registerPluginMessages } from '../index'

describe('shell i18n', () => {
  it('resolves shell chrome messages in both locales', () => {
    setLocale('zh-CN')
    expect(i18n.global.t('shell.logout')).toBe('退出登录')
    setLocale('en-US')
    expect(i18n.global.t('shell.logout')).toBe('Sign out')
  })

  it('setLocale persists the choice to localStorage', () => {
    setLocale('zh-CN')
    expect(getLocale()).toBe('zh-CN')
    expect(localStorage.getItem('hnb.locale')).toBe('zh-CN')
  })

  it('registers plugin messages under an isolated namespace', () => {
    registerPluginMessages('demo', {
      'zh-CN': { title: '演示插件' },
      'en-US': { title: 'Demo Plugin' },
    })
    setLocale('zh-CN')
    expect(i18n.global.t('demo.title')).toBe('演示插件')
    setLocale('en-US')
    expect(i18n.global.t('demo.title')).toBe('Demo Plugin')
  })
})
