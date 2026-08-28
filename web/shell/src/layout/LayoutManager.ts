/**
 * LayoutManager — per V3.6 §3 (Layout Manager)
 *
 * Holds the reactive menu state produced by NavigationManager.loadMenus().
 * Layout components subscribe to `menus` ref for re-rendering.
 *
 * The LayoutManager does NOT generate menus from Manifests (V3.6 §2.1)
 * and does NOT perform authoritative permission filtering (V3.6 §4.1).
 */

import { ref, type Ref } from 'vue'
import type { MenuGroup, MenuItem } from '@hnb/types'

export class LayoutManager {
  private _menus: Ref<MenuGroup[]> = ref([])

  /**
   * Update the menu state. Called by App.vue after navigation loads.
   */
  render(menus: MenuGroup[]): void {
    this._menus.value = menus
  }

  /**
   * Reactive reference the layout can subscribe to.
   */
  get menus(): Ref<MenuGroup[]> {
    return this._menus
  }

  getAllItems(): MenuItem[] {
    const items: MenuItem[] = []
    for (const group of this._menus.value) {
      items.push(...this.flattenItems(group.items))
    }
    return items
  }

  private flattenItems(items: MenuItem[]): MenuItem[] {
    const result: MenuItem[] = []
    for (const item of items) {
      result.push(item)
      if (item.children) {
        result.push(...this.flattenItems(item.children))
      }
    }
    return result
  }

  clear(): void {
    this._menus.value = []
  }
}

let _layoutManager: LayoutManager | null = null

export function getLayoutManager(): LayoutManager {
  if (!_layoutManager) {
    _layoutManager = new LayoutManager()
  }
  return _layoutManager
}

export default getLayoutManager
