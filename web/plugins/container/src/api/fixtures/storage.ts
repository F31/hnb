import type { PersistentVolume, PersistentVolumeClaim, StorageClassInfo } from '../storageApi'

const accessModes = ['ReadWriteOnce', 'ReadWriteMany', 'ReadOnlyMany'] as const

export const persistentVolumesFixture: PersistentVolume[] = Array.from({ length: 212 }, (_, index) => {
  const number = index + 1
  const bound = number % 3 !== 0
  return {
    name: number === 1 ? 'pv-nfs-production-data' : `pv-storage-${String(number).padStart(3, '0')}`,
    capacity: `${number % 5 === 0 ? 100 : number % 2 === 0 ? 50 : 20}Gi`,
    status: bound ? 'Bound' : 'Released',
    accessModes: [accessModes[number % accessModes.length]],
    reclaimPolicy: 'Retain',
    claimName: bound ? `data-service-${String(number).padStart(2, '0')}` : '',
    claimNamespace: bound ? (number % 4 === 0 ? 'production' : 'default') : '',
    service: bound ? `service-${String(number).padStart(2, '0')}` : '',
    createdAt: new Date(Date.UTC(2026, 6, 1 + (index % 30), 8, index % 60)).toISOString(),
    labels: { environment: number % 4 === 0 ? 'production' : 'default', storage: 'nfs' },
    storageClassName: number % 2 === 0 ? 'sc-nfs-standard' : 'sc-unicloud-sds',
  }
})

export const persistentVolumeClaimsFixture: PersistentVolumeClaim[] = Array.from({ length: 40 }, (_, index) => {
  const number = index + 1
  return {
    name: number === 1 ? 'mysql-data' : `pvc-application-${String(number).padStart(2, '0')}`,
    namespace: 'default',
    status: number % 5 === 0 ? 'Pending' : 'Bound',
    capacity: `${number % 3 === 0 ? 50 : 20}Gi`,
    accessModes: [accessModes[number % 2]],
    storageClassName: number % 2 === 0 ? 'sc-nfs-standard' : 'sc-unicloud-sds',
    volumeName: `pv-storage-${String(number).padStart(3, '0')}`,
    service: number % 5 === 0 ? '' : `application-${String(number).padStart(2, '0')}`,
    createdAt: new Date(Date.UTC(2026, 7, 1 + (index % 8), 9, index % 60)).toISOString(),
    labels: { app: `application-${number}` },
  }
})

export const storageClassesFixture: StorageClassInfo[] = [
  {
    name: 'sc-nfs-standard',
    provisioner: 'unicloud-sds-fs',
    reclaimPolicy: 'Retain',
    allowVolumeExpansion: true,
    poolPolicy: '',
    createdAt: '2026-07-01T08:00:00Z',
    parameters: { server: '10.10.10.20', path: '/data/nfs', mountOptions: 'nfsvers=4.1' },
    labels: { tier: 'standard' },
  },
  {
    name: 'sc-iscsi-ec42',
    provisioner: 'iscsi.csi.unicloud.com',
    reclaimPolicy: 'Retain',
    allowVolumeExpansion: true,
    poolPolicy: '纠删码4+2:1',
    createdAt: '2026-07-03T10:20:00Z',
    parameters: { pool: 'ec-pool-42', fsType: 'ext4' },
    labels: { tier: 'performance' },
  },
  {
    name: 'sc-unicloud-sds',
    provisioner: 'unicloud-sds-fs',
    reclaimPolicy: 'Delete',
    allowVolumeExpansion: true,
    poolPolicy: '副本3:1',
    createdAt: '2026-07-05T12:00:00Z',
    parameters: { pool: 'sds-default' },
    labels: { tier: 'general' },
  },
  {
    name: 'sc-archive',
    provisioner: 'nfs.csi.k8s.io',
    reclaimPolicy: 'Retain',
    allowVolumeExpansion: false,
    poolPolicy: '',
    createdAt: '2026-07-08T15:30:00Z',
    parameters: { server: 'archive.hnb.local', share: '/archive' },
    labels: { tier: 'archive' },
  },
]
