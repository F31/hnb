/**
 * CNI 能力探测 service adapter。
 * 依据插件市场目录中已安装的 CNI 插件推导能力总览（开发 fixture；生产空态）。
 */
import type {
  CniCapabilityMatrix,
  CniCapabilityOverview,
  CniFeature,
  CniName,
} from '../types/capability'
import { isCniCapabilityAvailable } from '../types/capability'
import { cniCapabilityMatricesFixture } from './fixtures/capability'
import { getPluginMarketCatalog } from './p4Api'

const CNI_NAMES: CniName[] = ['kube-ovn', 'cilium', 'calico']

/** 能力总览：全部 CNI 矩阵 + 已安装 CNI（由插件市场目录推导） */
export async function getCniCapabilityOverview(): Promise<CniCapabilityOverview> {
  const catalog = await getPluginMarketCatalog()
  const installed = catalog.find((p) => CNI_NAMES.includes(p.name as CniName) && p.installed)
  const installedCni = installed ? (installed.name as CniName) : null

  const cnis = installedCni
    ? cniCapabilityMatricesFixture
    : catalog.length > 0
      ? cniCapabilityMatricesFixture
      : []

  return { cnis, installedCni }
}

/** 取指定 CNI 的能力矩阵（未收录返回 null） */
export function matrixForCni(cnis: CniCapabilityMatrix[], cni: CniName): CniCapabilityMatrix | null {
  return cnis.find((m) => m.cni === cni) ?? null
}

/** 判定某个 CNI 是否具备某项特性（供容器层功能显隐使用） */
export function cniHasCapability(
  matrix: CniCapabilityMatrix | null,
  feature: CniFeature,
): boolean {
  if (!matrix) return false
  return isCniCapabilityAvailable(matrix.capabilities[feature])
}
