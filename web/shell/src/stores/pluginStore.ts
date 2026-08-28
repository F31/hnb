import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import type { PluginInstance } from '@hnb/types'

export const usePluginStore = defineStore('plugin', () => {
  // Use reactive Map by wrapping its contents explicitly; we trigger reactivity
  // via Array.from iteration in getters. For mutation, we replace the Map ref.
  const plugins = ref<Map<string, PluginInstance>>(new Map())
  const loading = ref<string[]>([])
  const errors = ref<Map<string, Error>>(new Map())
  const activated = ref<string[]>([])

  function add(plugin: PluginInstance): void {
    const next = new Map(plugins.value)
    next.set(plugin.name!, plugin)
    plugins.value = next
  }

  function get(pluginId: string): PluginInstance | undefined {
    return plugins.value.get(pluginId)
  }

  function getAll(): PluginInstance[] {
    return Array.from(plugins.value.values())
  }

  const getAllActive = computed<PluginInstance[]>(() => {
    return getAll().filter((p) => activated.value.includes(p.name!))
  })

  function isActivated(pluginId: string): boolean {
    return activated.value.includes(pluginId)
  }

  function activate(pluginId: string): void {
    if (!activated.value.includes(pluginId)) {
      activated.value = [...activated.value, pluginId]
    }
  }

  function deactivate(pluginId: string): void {
    activated.value = activated.value.filter((id) => id !== pluginId)
  }

  function hasError(pluginId: string): boolean {
    return errors.value.has(pluginId)
  }

  function getError(pluginId: string): Error | undefined {
    return errors.value.get(pluginId)
  }

  function setError(pluginId: string, error: Error): void {
    const next = new Map(errors.value)
    next.set(pluginId, error)
    errors.value = next
  }

  function clearErrors(): void {
    errors.value = new Map()
  }

  function setLoading(pluginId: string, loadingNow: boolean): void {
    if (loadingNow) {
      if (!loading.value.includes(pluginId)) {
        loading.value = [...loading.value, pluginId]
      }
    } else {
      loading.value = loading.value.filter((id) => id !== pluginId)
    }
  }

  function isLoading(pluginId: string): boolean {
    return loading.value.includes(pluginId)
  }

  const loadingCount = computed(() => loading.value.length)

  function clear(): void {
    plugins.value = new Map()
    loading.value = []
    errors.value = new Map()
    activated.value = []
  }

  return {
    plugins,
    loading,
    errors,
    activated,
    getAllActive,
    loadingCount,
    add,
    get,
    getAll,
    isActivated,
    activate,
    deactivate,
    hasError,
    getError,
    setError,
    clearErrors,
    setLoading,
    isLoading,
    clear,
  }
})
