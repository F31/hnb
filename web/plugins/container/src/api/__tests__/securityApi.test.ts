import { describe, expect, it } from 'vitest'
import { vi } from 'vitest'
import { getClusterProtectionTopology, getSecurityDashboard, getVulnerabilityScanProjects, recentSecurityDates, setContainerSecurityClient, uploadVulnerabilityDatabase } from '../securityApi'

describe('securityApi dashboard', () => {
  it('builds a rolling seven-day date range', () => {
    expect(recentSecurityDates(new Date(2026, 7, 9, 12))).toEqual([
      '2026-08-03', '2026-08-04', '2026-08-05', '2026-08-06', '2026-08-07', '2026-08-08', '2026-08-09',
    ])
  })

  it('returns a complete production-safe dashboard shape', async () => {
    const data = await getSecurityDashboard()
    expect(data.trend).toHaveLength(7)
    expect(data.images).toEqual(expect.objectContaining({ private: expect.any(Number), public: expect.any(Number) }))
    expect(data.vulnerabilities).toEqual(expect.objectContaining({ critical: expect.any(Number), unknown: expect.any(Number) }))
  })

  it('derives cluster version, platform and node roles from Kubernetes', async () => {
    const get = vi.fn()
      .mockResolvedValueOnce({ gitVersion: 'v1.31.1' })
      .mockResolvedValueOnce({ items: [
        { metadata: { labels: { 'node-role.kubernetes.io/control-plane': '' } }, status: { nodeInfo: { architecture: 'amd64', osImage: 'UniOS V1' } } },
        { metadata: { labels: {} }, status: { nodeInfo: { architecture: 'amd64', osImage: 'UniOS V1' } } },
      ] })
    setContainerSecurityClient({ get, post: vi.fn(), put: vi.fn(), patch: vi.fn(), delete: vi.fn() } as any)
    await expect(getClusterProtectionTopology('cluster-a')).resolves.toEqual({ version: 'v1.31.1', architecture: 'AMD64', operatingSystem: 'UniOS V1', controlPlaneNodes: 1, workerNodes: 1 })
  })

  it('returns a safe project list and rejects non-tgz database files', async () => {
    await expect(getVulnerabilityScanProjects()).resolves.toEqual([])
    await expect(uploadVulnerabilityDatabase(new File(['invalid'], 'database.zip'))).rejects.toThrow('.tgz')
  })
})
