<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  /** 采样点，按时间先后排列 */
  points: number[]
  height?: number
  color?: string
  /** 纵轴上限，缺省按数据自适应 */
  max?: number
}>()

const height = computed(() => props.height ?? 56)
const color = computed(() => props.color ?? '#38bdf8')

const geometry = computed(() => {
  const data = props.points
  if (data.length < 2) return { line: '', area: '', max: 0 }

  const top = Math.max(props.max ?? 0, ...data, 1)
  const width = 100
  const step = width / (data.length - 1)
  const toY = (value: number) => {
    const ratio = Math.min(1, Math.max(0, value / top))
    return 100 - ratio * 100
  }

  const coords = data.map((value, index) => `${(index * step).toFixed(2)},${toY(value).toFixed(2)}`)
  const line = coords.join(' ')
  const area = `0,100 ${line} ${width},100`
  return { line, area, max: top }
})
</script>

<template>
  <div class="sparkline">
    <svg v-if="geometry.line" :viewBox="`0 0 100 100`" preserveAspectRatio="none" :height="height">
      <polygon :points="geometry.area" :fill="color" fill-opacity="0.12" />
      <polyline
        :points="geometry.line"
        fill="none"
        :stroke="color"
        stroke-width="1.6"
        stroke-linejoin="round"
        stroke-linecap="round"
        vector-effect="non-scaling-stroke"
      />
    </svg>
    <div v-else class="sparkline-empty">采样中…</div>
    <div v-if="geometry.line" class="sparkline-max">峰值 {{ Math.round(geometry.max) }}</div>
  </div>
</template>

<style scoped>
.sparkline {
  position: relative;
  width: 100%;
}

.sparkline svg {
  display: block;
  width: 100%;
}

.sparkline-empty {
  font-size: 12px;
  color: var(--dim);
  padding: 14px 0;
}

.sparkline-max {
  position: absolute;
  top: 0;
  right: 0;
  font-size: 10.5px;
  color: var(--dim);
}
</style>
