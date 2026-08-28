<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

interface FlatMenuItem {
  title: string
  /** 父级分组项可能为空路径，仅携带 children */
  path?: string
  icon?: string
  permission?: string
  children?: FlatMenuItem[]
}

const props = defineProps<{ menuItems: FlatMenuItem[] }>()
const router = useRouter()
const route = useRoute()
const expandedKeys = ref(new Set<string>())
const collapsedKeys = ref(new Set<string>())

const displayItems = computed<FlatMenuItem[]>(() => props.menuItems)

function itemKey(item: FlatMenuItem): string {
  return item.path || item.title
}

function itemMatches(item: FlatMenuItem): boolean {
  if (item.path && route.path.startsWith(item.path)) return true
  return item.children?.some((child) => itemMatches(child)) ?? false
}

function isExpanded(item: FlatMenuItem): boolean {
  const key = itemKey(item)
  if (collapsedKeys.value.has(key)) return false
  return expandedKeys.value.has(key) || itemMatches(item)
}

function handleItemClick(item: FlatMenuItem) {
  if (item.children?.length) {
    const key = itemKey(item)
    const expanded = new Set(expandedKeys.value)
    const collapsed = new Set(collapsedKeys.value)
    if (isExpanded(item)) {
      expanded.delete(key)
      collapsed.add(key)
    } else {
      collapsed.delete(key)
      expanded.add(key)
    }
    expandedKeys.value = expanded
    collapsedKeys.value = collapsed
  } else if (item.path) {
    router.push(item.path)
  }
}
</script>

<template>
  <ul class="menu-list">
    <li
      v-for="item in displayItems"
      :key="item.path"
      :class="{
        'menu-item': true,
        'has-children': item.children && item.children.length > 0,
        expanded: isExpanded(item),
      }"
      :role="item.children?.length ? 'button' : undefined"
      :tabindex="item.children?.length ? 0 : undefined"
      :aria-expanded="item.children?.length ? isExpanded(item) : undefined"
      @click.stop="handleItemClick(item)"
      @keydown.enter.prevent.stop="handleItemClick(item)"
      @keydown.space.prevent.stop="handleItemClick(item)"
    >
      <div class="menu-row">
        <span class="menu-label">
          <span v-if="item.icon" class="menu-icon" aria-hidden="true"></span>
          {{ item.title }}
        </span>
        <span
          v-if="item.children && item.children.length > 0"
          class="menu-arrow"
          aria-hidden="true"
        >
          ›
        </span>
      </div>
      <RecursiveMenu
        v-if="item.children && item.children.length > 0 && isExpanded(item)"
        :menu-items="item.children as unknown as FlatMenuItem[]"
        class="menu-sublist"
      />
    </li>
  </ul>
</template>

<script lang="ts">
import { defineComponent } from 'vue'

// Self-reference support for recursive rendering.
// Reference the SFC explicitly to avoid circular setup block imports.
export default defineComponent({
  name: 'RecursiveMenu',
})
</script>

<style scoped>
.menu-list {
  list-style: none;
  margin: 0;
  padding: 0;
}
.menu-item {
  cursor: pointer;
  color: #bac3d0;
  font-size: 14px;
  padding: 8px 12px;
  border-radius: 6px;
  transition: background 0.15s ease, color 0.15s ease;
}
.menu-item:hover { background: #1f2833; color: #fff; }
.menu-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
}
.menu-label {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 1;
}
.menu-icon {
  width: 6px;
  height: 6px;
  border-radius: 999px;
  background: #6f7d91;
  flex: 0 0 auto;
}
.menu-arrow {
  font-size: 10px;
  color: #6b7a8a;
  transition: transform 0.2s ease;
}
.menu-item.has-children.expanded > .menu-row .menu-arrow {
  transform: rotate(90deg);
}
.menu-sublist {
  margin-top: 4px;
  list-style: none;
  padding-left: 8px;
}
</style>
