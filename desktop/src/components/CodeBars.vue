<script setup lang="ts">
import { computed } from 'vue'
import { codeClass, formatNumber, sortedCodes } from '../utils/format'

const props = defineProps<{
  codes?: Record<string, number>
  total: number
  /** 空数据时的提示 */
  emptyHint?: string
}>()

interface Row {
  code: string
  count: number
  percent: number
  tone: string
}

const rows = computed<Row[]>(() => {
  const entries = sortedCodes(props.codes)
  const base = entries.reduce((sum, [, count]) => sum + count, 0) || props.total || 0
  return entries.map(([code, count]) => ({
    code,
    count,
    percent: base > 0 ? (count / base) * 100 : 0,
    tone: toneOf(code),
  }))
})

function toneOf(code: string): string {
  const n = Number(code)
  if (n >= 200 && n < 300) return 'ok'
  if (n >= 300 && n < 400) return 'accent'
  if (n >= 400 && n < 500) return 'warn'
  if (n >= 500) return 'err'
  return ''
}
</script>

<template>
  <div v-if="rows.length === 0" class="bars-empty">
    {{ emptyHint ?? '还没有状态码数据' }}
  </div>
  <div v-else class="bars">
    <div v-for="row in rows" :key="row.code" class="bar-row">
      <span :class="codeClass(Number(row.code))">{{ row.code }}</span>
      <div class="bar-track">
        <div
          class="bar-fill"
          :class="row.tone"
          :style="{ width: `${Math.max(row.percent, 0.6)}%` }"
        />
      </div>
      <span class="bar-count">{{ formatNumber(row.count) }}</span>
    </div>
  </div>
</template>

<style scoped>
.bars-empty {
  font-size: 12px;
  color: var(--dim);
  padding: 6px 0;
}
</style>
