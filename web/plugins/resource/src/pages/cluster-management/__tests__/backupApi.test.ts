/**
 * backupApi service adapter 单元测试（生产路径空态 + 写操作抛未开放）。
 */
import { describe, it, expect, vi, afterEach } from 'vitest'

const OLD_ENV = { ...import.meta.env }

afterEach(() => {
  vi.resetModules()
  Object.assign(import.meta.env, OLD_ENV)
})

describe('backupApi 生产路径（无 fixture 标志）', () => {
  it('四类列表均返回空态', async () => {
    const mod = await import('../api/backupApi')
    expect(await mod.getBackupPolicies('')).toEqual([])
    expect(await mod.getBackupTasks('')).toEqual([])
    expect(await mod.getRestoreTasks('')).toEqual([])
    expect(await mod.getBackupRepositories('')).toEqual([])
  })

  it('写操作抛未开放', async () => {
    const mod = await import('../api/backupApi')
    await expect(mod.createBackupPolicy('', {} as never)).rejects.toThrow(/Unavailable/)
    await expect(mod.deleteBackupPolicy('', 'x')).rejects.toThrow(/Unavailable/)
    await expect(mod.createRestoreTask('', {} as never)).rejects.toThrow(/Unavailable/)
    await expect(mod.createBackupRepository('', {} as never)).rejects.toThrow(/Unavailable/)
  })
})
