/**
 * SchemaEngine — 校验与标准化服务端 UI Schema（V2.5 §4 / §20）。
 *
 * 错误兼容策略（V2.5 §4.4）：
 *  - 未识别的可选字段：忽略；
 *  - Schema 版本不兼容：抛出 SchemaError('INCOMPATIBLE')，由渲染层展示升级提示；
 *  - 缺必需字段：抛出 SchemaError('INVALID')。
 */

import type { PageSchema, SchemaEnvelope } from './types'

export const SUPPORTED_API_VERSIONS = ['ui.hnb.io/v1'] as const

/** 当前 Shell 支持的 Schema 契约版本，随 Shell 发布更新 */
export const SHELL_VERSION = '2.5.0'

export type SchemaErrorCode = 'INVALID' | 'UNSUPPORTED_API_VERSION' | 'INCOMPATIBLE'

export class SchemaError extends Error {
  readonly code: SchemaErrorCode
  constructor(code: SchemaErrorCode, message: string) {
    super(message)
    this.name = 'SchemaError'
    this.code = code
  }
}

export function compareVersion(a: string, b: string): number {
  const pa = a.split('.').map((n) => parseInt(n, 10) || 0)
  const pb = b.split('.').map((n) => parseInt(n, 10) || 0)
  for (let i = 0; i < Math.max(pa.length, pb.length); i++) {
    const diff = (pa[i] ?? 0) - (pb[i] ?? 0)
    if (diff !== 0) return diff
  }
  return 0
}

export class SchemaEngine {
  /** 服务端声明各 Schema id 支持的最高 revision（fail-closed 上限） */
  private maxRevisions = new Map<string, number>()

  /** 声明某 Schema id 当前支持的最高 revision；高于此值的 Schema 被拒绝 */
  declareSupportedRevision(schemaID: string, revision: number): void {
    if (!schemaID || !Number.isInteger(revision) || revision < 0) {
      throw new Error('invalid supported revision')
    }
    this.maxRevisions.set(schemaID, revision)
  }

  /**
   * 校验统一信封：apiVersion / kind / metadata.id / metadata.revision 必填，
   * apiVersion 必须在支持列表内，minShellVersion 不得高于当前 Shell，
   * revision 不得超过服务端声明的支持上限。
   */
  validateEnvelope<S>(schema: SchemaEnvelope<S>, expectedKind?: string): SchemaEnvelope<S> {
    if (!schema || typeof schema !== 'object') {
      throw new SchemaError('INVALID', 'Schema is not an object')
    }
    if (!schema.apiVersion) {
      throw new SchemaError('INVALID', 'metadata.apiVersion is required')
    }
    if (!(SUPPORTED_API_VERSIONS as readonly string[]).includes(schema.apiVersion)) {
      throw new SchemaError(
        'UNSUPPORTED_API_VERSION',
        `Unsupported apiVersion: ${schema.apiVersion}`,
      )
    }
    if (!schema.kind || (expectedKind && schema.kind !== expectedKind)) {
      throw new SchemaError(
        'INVALID',
        `kind mismatch: expected ${expectedKind ?? 'non-empty'}, got ${schema.kind}`,
      )
    }
    if (!schema.metadata?.id) {
      throw new SchemaError('INVALID', 'metadata.id is required')
    }
    if (typeof schema.metadata.revision !== 'number' || schema.metadata.revision < 0) {
      throw new SchemaError('INVALID', 'metadata.revision must be a non-negative number')
    }
    const minShell = schema.metadata.minShellVersion
    if (minShell && compareVersion(minShell, SHELL_VERSION) > 0) {
      throw new SchemaError(
        'INCOMPATIBLE',
        `Schema requires Shell >= ${minShell}, current ${SHELL_VERSION}`,
      )
    }
    const maxRevision = this.maxRevisions.get(schema.metadata.id)
    if (maxRevision !== undefined && schema.metadata.revision > maxRevision) {
      throw new SchemaError(
        'INCOMPATIBLE',
        `Schema "${schema.metadata.id}" revision ${schema.metadata.revision} exceeds supported revision ${maxRevision}`,
      )
    }
    return schema
  }

  validatePageSchema(schema: PageSchema): PageSchema {
    this.validateEnvelope(schema, 'PageSchema')
    if (!schema.spec || !Array.isArray(schema.spec.regions)) {
      throw new SchemaError('INVALID', 'spec.regions must be an array')
    }
    return schema
  }
}

export function createSchemaEngine(): SchemaEngine {
  return new SchemaEngine()
}
