import { describe, expect, it } from 'vitest'
import { ComponentRegistry } from '../ComponentRegistry'
import { registerBuiltinComponents } from '../builtins'

describe('registerBuiltinComponents', () => {
  it('registers UI Kit baseline primitives', () => {
    const registry = new ComponentRegistry()
    registerBuiltinComponents(registry)

    for (const type of ['PageShell', 'Toolbar', 'Button', 'TableActions', 'SelectInput', 'DateInput', 'FormField', 'DetailPanel', 'ActionBar']) {
      expect(registry.has(type)).toBe(true)
    }
  })

  it('validates baseline primitive props', () => {
    const registry = new ComponentRegistry()
    registerBuiltinComponents(registry)

    expect(registry.validateProps('PageShell', { title: 'Workloads' }).valid).toBe(true)
    expect(registry.validateProps('PageShell', { description: 'missing title' }).valid).toBe(false)
    expect(registry.validateProps('Button', { variant: 'unsafe' }).valid).toBe(false)
    expect(registry.validateProps('SelectInput', { options: [] }).valid).toBe(true)
  })
})
