<script setup lang="ts">
import { computed } from 'vue'
import type { Latency } from '../api/types'
import { formatMs } from '../utils/format'

const props = defineProps<{ latency: Latency }>()

interface Row {
  label: string
  value: number
  percent: number
  tone: string
}

const rows = computed<Row[]>(() => {
  const max = Math.max(props.latency.max || 0, 1)
  const item = (label: string, value: number, tone: string): Row => ({
    label,
    value,
    percent: Math.max((value / max) * 100, value > 0 ? 1.5 : 0),
    tone,
  })
  return [
    item('最快', props.latency.min, 'ok'),
    item('P50', props.latency.p50, 'accent'),
    item('P95', props.latency.p95, 'warn'),
    item('P99', props.latency.p99, 'err'),
    item('最慢', props.latency.max, 'err'),
  ]
})
</script>

<template>
  <div class="bars">
    <div v-for="row in rows" :key="row.label" class="bar-row latency-row">
      <span class="latency-label">{{ row.label }}</span>
      <div class="bar-track">
        <div class="bar-fill" :class="row.tone" :style="{ width: `${row.percent}%` }" />
      </div>
      <span class="bar-count">{{ formatMs(row.value) }}</span>
    </div>
  </div>
</template>

<style scoped>
.latency-row {
  grid-template-columns: 40px 1fr 74px;
}

.latency-label {
  color: var(--muted);
  font-size: 12px;
}
</style>
