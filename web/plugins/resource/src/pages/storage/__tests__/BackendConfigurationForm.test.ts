import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { createComponentRegistry } from '@hnb/schema-engine'
import BackendConfigurationForm from '../components/BackendConfigurationForm.vue'
import { BACKEND_CONFIGURATION_COMPONENT, registerStorageComponents } from '../runtime/registerStorageComponents'
import { createTestI18n } from '../../cluster-management/__tests__/testUtils'

const schema = {
  schemaVersion: '1.0.0' as const,
  providerType: 'nfs',
  providerSchemaVersion: '1.0.0',
  componentType: BACKEND_CONFIGURATION_COMPONENT as 'resource.storage.BackendConfigurationForm',
  fields: [
    { name: 'server', label: 'NFS server', type: 'text' as const, required: true },
    { name: 'readOnly', label: 'Read only', type: 'boolean' as const, required: false },
  ],
}

describe('trusted backend configuration form', () => {
  it('registers one local component and rejects untrusted component names', () => {
    const registry = createComponentRegistry()
    registerStorageComponents(registry)
    expect(registry.resolve(BACKEND_CONFIGURATION_COMPONENT)).toBe(BackendConfigurationForm)
    expect(registry.resolve('https://evil.example/form.js')).toBeNull()
    expect(registry.validateProps(BACKEND_CONFIGURATION_COMPONENT, { schema, script: 'alert(1)' }).valid).toBe(false)
  })

  it('emits common typed fields, provider attributes, and references without secret values', async () => {
    const wrapper = mount(BackendConfigurationForm, { props: { schema }, global: { plugins: [createTestI18n('en-US')] } })
    await wrapper.get('#storage-backend-id').setValue('nfs-primary')
    await wrapper.get('#storage-backend-name').setValue('Primary NFS')
    await wrapper.get('#storage-secret-provider').setValue('platform-secrets')
    await wrapper.get('#storage-secret-scope').setValue('tenant:tenant-a')
    await wrapper.get('#storage-secret-name').setValue('nfs-primary')
    await wrapper.get('#storage-attribute-server').setValue('nfs.internal')
    await wrapper.get('#storage-attribute-readOnly').setValue(true)
    await wrapper.get('form').trigger('submit')

    const payload = wrapper.emitted('submit')?.[0]?.[0]
    expect(payload).toMatchObject({
      providerType: 'nfs',
      providerSchemaVersion: '1.0.0',
      secretReference: { provider: 'platform-secrets', scope: 'tenant:tenant-a', name: 'nfs-primary' },
      attributes: { server: 'nfs.internal', readOnly: true },
    })
    expect(JSON.stringify(payload)).not.toMatch(/secretValue|password|token/)
  })

  it('localizes trusted schema fields without changing submitted values', () => {
    const wrapper = mount(BackendConfigurationForm, { props: { schema }, global: { plugins: [createTestI18n('zh-CN')] } })

    expect(wrapper.text()).toContain('后端标识')
    expect(wrapper.text()).toContain('NFS 服务器')
    expect(wrapper.text()).toContain('只读')
    expect(wrapper.text()).toContain('创建存储后端')
    expect(wrapper.text()).not.toContain('Backend ID')
    expect(wrapper.text()).not.toContain('NFS server')
  })
})
