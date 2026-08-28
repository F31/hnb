<template>
  <div ref="containerEl" class="hnb-virtual-list" :style="{ height: height ? height + 'px' : '100%' }" @scroll="onScroll">
    <div class="hnv-virtual-list-spacer" :style="{ height: totalHeight + 'px' }">
      <div class="hnb-virtual-list-items" :style="{ transform: `translateY(${offsetY}px)` }">
        <div
          v-for="(item, i) in visibleItems"
          :key="getItemKey(item, i)"
          class="hnb-virtual-list-item"
          :style="{ height: itemHeight + 'px' }"
        >
          <slot name="item" :item="item" :index="visibleStart + i" />
        </div>
      </div>
    </div>
    <div v-if="loading" class="hnb-virtual-list-loading">
      <slot name="loading">
        <span>加载中...</span>
      </slot>
    </div>
    <div v-if="!loading && data.length === 0" class="hnb-virtual-list-empty">
      <slot name="empty">
        <span>暂无数据</span>
      </slot>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'

const props = withDefaults(defineProps<{
  data: unknown[]
  itemHeight: number
  height?: number
  rowKey?: string
  loading?: boolean
  overscan?: number
}>(), {
  overscan: 5,
  loading: false,
})

const emit = defineEmits<{
  endReached: []
}>()

const containerEl = ref<HTMLDivElement | null>(null)
const scrollTop = ref(0)
const containerHeight = ref(0)

function getItemKey(item: unknown, index: number): string | number | symbol {
  if (!props.rowKey || typeof item !== 'object' || item === null) return index

  const value = (item as Record<string, unknown>)[props.rowKey]
  return typeof value === 'string' || typeof value === 'number' || typeof value === 'symbol'
    ? value
    : index
}

const visibleStart = computed(() => {
  return Math.max(0, Math.floor(scrollTop.value / props.itemHeight) - props.overscan)
})

const visibleEnd = computed(() => {
  return Math.min(props.data.length, Math.ceil((scrollTop.value + containerHeight.value) / props.itemHeight) + props.overscan)
})

const visibleItems = computed(() => {
  return props.data.slice(visibleStart.value, visibleEnd.value)
})

const offsetY = computed(() => visibleStart.value * props.itemHeight)
const totalHeight = computed(() => props.data.length * props.itemHeight)

const nearBottom = computed(() => {
  return scrollTop.value + containerHeight.value >= props.data.length * props.itemHeight - props.itemHeight * 2
})

let lastEndReachedLength = -1

watch([nearBottom, () => props.loading, () => props.data.length], ([isNearBottom, isLoading, length]) => {
  if (isNearBottom && !isLoading && length > 0 && length !== lastEndReachedLength) {
    lastEndReachedLength = length
    emit('endReached')
  }
})

let resizeObserver: ResizeObserver | null = null

onMounted(() => {
  if (containerEl.value) {
    containerHeight.value = containerEl.value.clientHeight
    resizeObserver = new ResizeObserver(() => {
      containerHeight.value = containerEl.value?.clientHeight ?? 0
    })
    resizeObserver.observe(containerEl.value)
  }
})

onUnmounted(() => {
  resizeObserver?.disconnect()
})

function onScroll() {
  if (containerEl.value) {
    scrollTop.value = containerEl.value.scrollTop
  }
}
</script>

<style scoped>
.hnb-virtual-list {
  overflow-y: auto;
  position: relative;
}
.hnv-virtual-list-spacer {
  position: relative;
}
.hnb-virtual-list-items {
  position: absolute;
  left: 0;
  right: 0;
  top: 0;
}
.hnb-virtual-list-item {
  overflow: hidden;
}
.hnb-virtual-list-loading,
.hnb-virtual-list-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 40px;
  color: var(--hnb-color-text-tertiary, #6b7a8a);
  font-size: 13px;
}
</style>
