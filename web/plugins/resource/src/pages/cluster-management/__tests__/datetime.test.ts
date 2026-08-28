/**
 * datetime 转换工具单元测试。
 */
import { describe, it, expect } from 'vitest'
import { isoToLocalInput, localInputToIso } from '../utils/datetime'

describe('isoToLocalInput', () => {
  it('ISO 转 datetime-local 本地格式（yyyy-MM-ddTHH:mm，无秒/时区后缀）', () => {
    // 固定用本地时区解析，避免 CI 时区漂移
    const local = new Date(2026, 7, 7, 0, 47) // 2026-08-07 00:47 本地
    const out = isoToLocalInput(local.toISOString())
    expect(out).toBe('2026-08-07T00:47')
    expect(out).not.toMatch(/Z|\.\d{3}/)
  })

  it('空值/非法值返回空串', () => {
    expect(isoToLocalInput('')).toBe('')
    expect(isoToLocalInput('not-a-date')).toBe('')
  })
})

describe('localInputToIso', () => {
  it('datetime-local 本地值转 ISO（往返一致）', () => {
    const iso = localInputToIso('2026-08-07T00:47')
    const back = isoToLocalInput(iso)
    expect(back).toBe('2026-08-07T00:47')
    expect(iso).toMatch(/^2026-08-0[67]T/)
  })

  it('空值返回空串', () => {
    expect(localInputToIso('')).toBe('')
  })
})
