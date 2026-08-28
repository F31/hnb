/**
 * useClusterCapabilities — CNI 能力探测 composable（模块级共享单例）。
 *
 * 供容器层/资源层功能显隐与置灰判断：根据已安装 CNI 插件推导能力总览，
 * 提供 hasCapability(feature) 便捷判定。模块级 ref 保证多页面共享一份加载状态。
 */
import { computed, ref } from 'vue'
import type { CniCapabilityOverview, CniFeature } from '../types/capability'
import { cniHasCapability, getCniCapabilityOverview, matrixForCni } from '../api/capabilityApi'

const overview = ref<CniCapabilityOverview | null>(null)
const loading = ref(false)
let loaded = false

const currentMatrix = computed(() => {
  const o = overview.value
  return o?.installedCni && o.cnis.length ? matrixForCni(o.cnis, o.installedCni) : null
})

export function useClusterCapabilities() {
  async function refresh(): Promise<void> {
    loading.value = true
    try {
      overview.value = await getCniCapabilityOverview()
      loaded = true
    } catch {
      overview.value = null
    } finally {
      loading.value = false
    }
  }

  function hasCapability(feature: CniFeature): boolean {
    return cniHasCapability(currentMatrix.value, feature)
  }

  function ensureLoaded(): Promise<void> {
    if (loaded) return Promise.resolve()
    return refresh()
  }

  return { overview, loading, hasCapability, refresh, ensureLoaded }
}
