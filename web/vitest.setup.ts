import { config } from '@vue/test-utils'
import { defineComponent, h } from 'vue'

const NDataTableStub = defineComponent({
  name: 'NDataTable',
  setup(_, { slots }) {
    return () => h('div', { 'data-test': 'n-data-table' }, slots.default?.())
  },
})

config.global.components = {
  ...config.global.components,
  'n-data-table': NDataTableStub,
}
