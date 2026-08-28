import { describe, it, expect } from 'vitest'
import { SchemaEngine, SchemaError } from '../SchemaEngine'
import type { PageSchema } from '../types'

function validPageSchema(): PageSchema {
  return {
    apiVersion: 'ui.hnb.io/v1',
    kind: 'PageSchema',
    metadata: { id: 'container.workloads.list', revision: 12 },
    spec: {
      template: 'list',
      regions: [
        { id: 'table', componentType: 'DataTable', span: 12, props: {} },
      ],
    },
  }
}

describe('SchemaEngine', () => {
  const engine = new SchemaEngine()

  it('合法 PageSchema 通过校验', () => {
    expect(engine.validatePageSchema(validPageSchema())).toBeTruthy()
  })

  it('缺 apiVersion 抛 INVALID', () => {
    const schema = validPageSchema() as any
    delete schema.apiVersion
    expect(() => engine.validatePageSchema(schema)).toThrowError(SchemaError)
  })

  it('不支持的 apiVersion 抛 UNSUPPORTED_API_VERSION', () => {
    const schema = validPageSchema()
    schema.apiVersion = 'ui.hnb.io/v99'
    try {
      engine.validatePageSchema(schema)
      expect.unreachable()
    } catch (err) {
      expect((err as SchemaError).code).toBe('UNSUPPORTED_API_VERSION')
    }
  })

  it('kind 不匹配抛 INVALID', () => {
    const schema = validPageSchema() as any
    schema.kind = 'FormSchema'
    expect(() => engine.validatePageSchema(schema)).toThrowError(/kind mismatch/)
  })

  it('缺 metadata.id / revision 抛 INVALID', () => {
    const s1 = validPageSchema() as any
    delete s1.metadata.id
    expect(() => engine.validatePageSchema(s1)).toThrowError(/metadata.id/)
    const s2 = validPageSchema() as any
    delete s2.metadata.revision
    expect(() => engine.validatePageSchema(s2)).toThrowError(/revision/)
  })

  it('minShellVersion 高于当前 Shell 抛 INCOMPATIBLE', () => {
    const schema = validPageSchema()
    schema.metadata.minShellVersion = '99.0.0'
    try {
      engine.validatePageSchema(schema)
      expect.unreachable()
    } catch (err) {
      expect((err as SchemaError).code).toBe('INCOMPATIBLE')
    }
  })

  it('spec.regions 非数组抛 INVALID', () => {
    const schema = validPageSchema() as any
    schema.spec.regions = {}
    expect(() => engine.validatePageSchema(schema)).toThrowError(/regions/)
  })
})
