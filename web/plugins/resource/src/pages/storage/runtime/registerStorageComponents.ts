import type { ComponentRegistry } from '@hnb/schema-engine'
import BackendConfigurationForm from '../components/BackendConfigurationForm.vue'

export const BACKEND_CONFIGURATION_COMPONENT = 'resource.storage.BackendConfigurationForm'

export function registerStorageComponents(registry: ComponentRegistry): void {
  registry.register({
    type: BACKEND_CONFIGURATION_COMPONENT,
    component: BackendConfigurationForm,
    version: '1.0.0',
    pluginId: 'resource',
    propsSchema: {
      type: 'object',
      required: ['schema'],
      additionalProperties: false,
      properties: {
        schema: { type: 'object' },
        submitting: { type: 'boolean' },
      },
    },
  })
}
