/**
 * YAML 校验工具（基于 `yaml` 包，纯前端不执行内容）。
 */
import { parseDocument } from 'yaml'
import { pluginT } from '../api/pluginI18n'

/** YAML 语法校验：返回错误摘要（null 表示合法）；空文本视为合法（由必填校验负责） */
export function validateYaml(text: string): string | null {
  if (!text.trim()) return null
  const doc = parseDocument(text)
  if (doc.errors.length > 0) {
    const first = doc.errors[0]
    return `${pluginT('resource.clusterMgmt.error.yamlInvalid')}${first.message}`
  }
  return null
}
