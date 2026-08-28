import { describe, expect, it } from 'vitest'
import { mapAccessIngress, mapAccessNetworkPolicy, mapAccessService, mapMetalLBPool, validIpRange } from '../accessApi'

describe('accessApi Kubernetes mappers', () => {
  it('maps multi-port Services', () => {
    expect(mapAccessService({
      metadata: { name: 'api', namespace: 'default' },
      spec: { type: 'NodePort', clusterIP: '10.96.0.10', ports: [{ name: 'http', port: 80, targetPort: 8080, protocol: 'TCP' }, { name: 'admin', port: 81, targetPort: 8081 }] },
    })).toMatchObject({ name: 'api', type: 'NodePort', clusterIp: '10.96.0.10', ports: [{ name: 'http', port: 80, targetPort: 8080 }, { name: 'admin', port: 81, targetPort: 8081 }] })
  })

  it('flattens Ingress hosts and paths', () => {
    expect(mapAccessIngress({ metadata: { name: 'route', namespace: 'argocd' }, spec: { rules: [{ host: 'app.example.com', http: { paths: [{ path: '/', backend: { service: { name: 'web', port: { number: 80 } } } }] } }] } }).rules[0])
      .toMatchObject({ host: 'app.example.com', path: '/', serviceName: 'web', servicePort: 80 })
  })

  it('maps MetalLB IPAddressPool ranges', () => {
    expect(mapMetalLBPool({ metadata: { name: 'pool' }, spec: { addresses: ['10.0.0.10-10.0.0.19'] } }))
      .toMatchObject({ name: 'pool', startIp: '10.0.0.10', endIp: '10.0.0.19', availableIps: 10 })
  })

  it('maps NetworkPolicy selectors and policy types', () => {
    expect(mapAccessNetworkPolicy({ metadata: { name: 'deny', namespace: 'argocd' }, spec: { podSelector: { matchLabels: { app: 'web' } }, policyTypes: ['Ingress'] } }))
      .toMatchObject({ name: 'deny', namespace: 'argocd', policyTypes: ['Ingress'], matchLabels: { app: 'web' } })
  })

  it('validates IPv4 ranges and ordering', () => {
    expect(validIpRange('10.0.0.10', '10.0.0.20')).toBe(true)
    expect(validIpRange('10.0.0.20', '10.0.0.10')).toBe(false)
    expect(validIpRange('999.0.0.1', '999.0.0.2')).toBe(false)
  })
})
