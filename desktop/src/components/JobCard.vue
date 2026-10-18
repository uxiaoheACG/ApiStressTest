<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import type { JobSnapshot } from '../api/types'
import { progressPercent } from '../composables/useJobs'
import { formatMs, formatNumber, formatQps, formatTime, statusClass, statusText } from '../utils/format'
import CodeBars from './CodeBars.vue'

const props = defineProps<{
  job: JobSnapshot
  cancelling?: boolean
}>()

const emit = defineEmits<{ cancel: [string] }>()

const percent = computed(() => progressPercent(props.job))
const failures = computed(() => props.job.progress.failure ?? 0)
const running = computed(() => props.job.status === 'running' || props.job.status === 'pending')

const detailTo = computed(() => `/jobs/${props.job.id}`)

function cancel() {
  emit('cancel', props.job.id)
}
</script>

<template>
  <article class="job-card">
    <header class="job-head">
      <div class="job-title">
        <span :class="statusClass(job.status)">{{ statusText(job.status) }}</span>
        <span class="job-id mono">{{ job.id }}</span>
        <span class="job-time">开始于 {{ formatTime(job.startedAt) }}</span>
      </div>
      <div class="job-actions">
        <RouterLink :to="detailTo" class="btn btn-sm">详情</RouterLink>
        <button
          v-if="running"
          class="btn btn-danger btn-sm"
          :disabled="cancelling"
          @click="cancel"
        >
          {{ cancelling ? '取消中…' : '取消' }}
        </button>
      </div>
    </header>

    <div class="job-progress">
      <div class="split" style="margin-bottom: 6px">
        <span class="progress-label">
          已完成 <strong>{{ formatNumber(job.progress.done) }}</strong> /
          {{ formatNumber(job.progress.total) }}
        </span>
        <span class="progress-percent">{{ percent.toFixed(1) }}%</span>
      </div>
      <div class="progress">
        <div
          class="progress-bar"
          :class="{ running }"
          :style="{ width: `${percent}%` }"
        />
      </div>
    </div>

    <div class="job-stats">
      <div class="stat">
        <div class="stat-label">成功</div>
        <div class="stat-value ok">{{ formatNumber(job.progress.success) }}</div>
      </div>
      <div class="stat">
        <div class="stat-label">失败合计</div>
        <div class="stat-value" :class="failures > 0 ? 'err' : ''">
          {{ formatNumber(failures) }}
        </div>
        <div class="stat-sub">
          超时 {{ formatNumber(job.progress.timeout) }} · 错误
          {{ formatNumber(job.progress.error) }} · 非 2xx {{ formatNumber(job.progress.unsuccess) }}
        </div>
      </div>
      <div class="stat">
        <div class="stat-label">实时 QPS</div>
        <div class="stat-value accent">{{ formatQps(job.progress.qps) }}</div>
      </div>
      <div class="stat">
        <div class="stat-label">耗时</div>
        <div class="stat-value">{{ formatMs(job.elapsedMs) }}</div>
      </div>
    </div>

    <div v-if="job.progress.statusCodes && Object.keys(job.progress.statusCodes).length" class="job-codes">
      <CodeBars :codes="job.progress.statusCodes" :total="job.progress.done" />
    </div>

    <div v-if="job.err" class="banner banner-err" style="margin: 12px 16px">
      <span>⚠️</span>
      <span>{{ job.err }}</span>
    </div>
  </article>
</template>

<style scoped>
.job-card {
  background: var(--panel);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 15px 16px;
  display: flex;
  flex-direction: column;
  gap: 13px;
}

.job-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}

.job-title {
  display: flex;
  align-items: center;
  gap: 9px;
  flex-wrap: wrap;
}

.job-id {
  font-size: 13px;
  font-weight: 600;
  letter-spacing: 0.4px;
}

.job-time {
  font-size: 11.5px;
  color: var(--dim);
}

.job-actions {
  display: flex;
  gap: 7px;
}

.progress-label {
  font-size: 12.5px;
  color: var(--muted);
}

.progress-label strong {
  color: var(--text);
  font-variant-numeric: tabular-nums;
}

.progress-percent {
  font-size: 13px;
  font-weight: 600;
  color: var(--accent);
  font-variant-numeric: tabular-nums;
}

.job-stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: 10px;
}

.job-codes {
  border-top: 1px solid var(--border);
  padding-top: 11px;
}
</style>
