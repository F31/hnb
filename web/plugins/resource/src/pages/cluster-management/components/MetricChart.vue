<script setup lang="ts">
/**
 * MetricChart — ECharts 折线图封装（UI 规范 V2.6 §8 图表规范）。
 *
 * - 多序列折线（图例横向换行）；
 * - tooltip 显示完整时间、序列名、值与单位；
 * - 单位按数量级自适应（B/s→KB/s→MB/s 等）；
 * - 容器 resize 触发图表 resize；
 * - 无数据时显示标准空态，不绘制误导性的零值曲线。
 */
import { onBeforeUnmount, onMounted, ref, watch, type PropType } from 'vue'
import * as echarts from 'echarts/core'
import { LineChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'
import type { MetricSeries } from '../types/clusterMonitoring'

echarts.use([LineChart, GridComponent, TooltipComponent, LegendComponent, CanvasRenderer])

const props = defineProps({
  title: { type: String, required: true },
  unit: { type: String, default: '%' },
  series: { type: Array as PropType<MetricSeries[]>, default: () => [] },
  height: { type: String, default: '260px' },
  emptyText: { type: String, default: '暂无监控数据' },
})

const el = ref<HTMLDivElement | null>(null)
let chart: echarts.ECharts | null = null
let observer: ResizeObserver | null = null

/** 按数量级自适应单位（B/s→KB/s→MB/s…；百分比固定不变） */
function formatUnitValue(value: number, unit: string): string {
  if (unit === '%') return `${value.toFixed(2)}%`
  if (unit === 'ms' || unit === 'μs') return `${value}${unit}`
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const base = unit.replace(/\/s$/, '').toUpperCase() === 'B' ? '' : unit
  let v = Math.abs(value)
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  const suffix = unit.includes('/s') ? `${units[i]}/s` : units[i]
  return `${base}${v.toFixed(2)} ${suffix}`
}

function buildOption() {
  const hasData = props.series.some((s) => s.points.length > 0)
  return {
    animation: false,
    tooltip: {
      trigger: 'axis',
      formatter: (params: Array<{ seriesName: string; value: [string, number] }>) => {
        if (!params.length) return ''
        const t = params[0].value[0]
        const lines = params.map((p) => `${p.seriesName}: ${formatUnitValue(p.value[1], props.unit)}`).join('<br/>')
        return `${t}<br/>${lines}`
      },
    },
    legend: { top: 0, type: 'scroll' },
    grid: { left: 48, right: 16, top: 32, bottom: 28 },
    xAxis: {
      type: 'time',
      axisLabel: { formatter: (v: number) => new Date(v).toLocaleTimeString() },
    },
    yAxis: {
      type: 'value',
      max: hasData && props.unit === '%' ? 100 : undefined,
      axisLabel: { formatter: (v: number) => `${v}${props.unit}` },
    },
    series: props.series.map((s) => ({
      name: s.name,
      type: 'line' as const,
      showSymbol: false,
      smooth: true,
      lineStyle: { width: 2 },
      data: s.points.map((p) => [p.timestamp, p.value]),
    })),
  }
}

function render(): void {
  if (!chart) return
  if (!props.series.some((s) => s.points.length > 0)) {
    chart.clear()
    return
  }
  chart.setOption(buildOption(), true)
}

onMounted(() => {
  if (!el.value) return
  chart = echarts.init(el.value)
  render()
  observer = new ResizeObserver(() => chart?.resize())
  observer.observe(el.value)
})

watch(() => props.series, render, { deep: true })

onBeforeUnmount(() => {
  observer?.disconnect()
  chart?.dispose()
  chart = null
})
</script>

<template>
  <div class="metric-chart">
    <header class="metric-chart__header">
      <h4 class="metric-chart__title">{{ title }}</h4>
      <span class="metric-chart__unit">{{ unit }}</span>
    </header>
    <div v-if="!series.some((s) => s.points.length > 0)" class="metric-chart__empty" role="status">
      {{ emptyText }}
    </div>
    <div v-else ref="el" class="metric-chart__canvas" :style="{ height }" role="img" :aria-label="title"></div>
  </div>
</template>

<style scoped>
.metric-chart {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.metric-chart__header {
  display: flex;
  align-items: baseline;
  gap: 8px;
}
.metric-chart__title {
  margin: 0;
  font-size: 14px;
  font-weight: 600;
  color: var(--hnb-color-text-primary, #12172a);
}
.metric-chart__unit {
  font-size: 12px;
  color: var(--hnb-color-text-tertiary, #8a94a3);
}
.metric-chart__canvas { width: 100%; }
.metric-chart__empty {
  height: 120px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--hnb-color-text-tertiary, #8a94a3);
  font-size: 13px;
  border: 1px dashed var(--hnb-color-border, #e2e7ef);
  border-radius: var(--hnb-radius-sm, 4px);
}
</style>
