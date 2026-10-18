<script setup lang="ts">
import { computed, ref } from 'vue'
import { RouterLink } from 'vue-router'
import JobCard from '../components/JobCard.vue'
import { ApiError, cancelJob } from '../api/client'
import type { JobSnapshot } from '../api/types'
import { activeJobs, finishedJobs, useJobPolling } from '../composables/useJobs'
import { formatMs, formatNumber, formatQps, formatTime, statusClass, statusText } from '../utils/format'

const { jobs, loading, error, refresh } = useJobPolling()

const cancellingId = ref('')
const cancelError = ref('')

const running = computed(() => activeJobs(jobs.value))
const finished = computed(() => finishedJobs(jobs.value).slice(0, 8))

const totals = computed(() => {
  let done = 0
  let success = 0
  let failure = 0
  for (const job of jobs.value) {
    done += job.progress?.done ?? 0
    success += job.progress?.success ?? 0
    failure += job.progress?.failure ?? 0
  }
  return { done, success, failure }
})

async function handleCancel(id: string) {
  cancellingId.value = id
  cancelError.value = ''
  try {
    await cancelJob(id)
    await refresh()
  } catch (err) {
    cancelError.value = err instanceof ApiError ? err.message : (err as Error).message
  } finally {
    cancellingId.value = ''
  }
}

function summaryOf(job: JobSnapshot) {
  return job.summary
}
</script>

<template>
  <div class="stack">
    <div v-if="cancelError" class="banner banner-err">
      <span>⚠️</span>
      <span>{{ cancelError }}</span>
    </div>

    <div v-if="error" class="banner banner-err">
      <span>⚠️</span>
      <span>{{ error }}</span>
      <div class="spacer" />
      <RouterLink to="/settings" class="link">检查后端地址</RouterLink>
    </div>

    <section class="overview">
      <div class="stat">
        <div class="stat-label">进行中的任务</div>
        <div class="stat-value accent">{{ running.length }}</div>
        <div class="stat-sub">后端上限 4 个</div>
      </div>
      <div class="stat">
        <div class="stat-label">累计完成请求</div>
        <div class="stat-value">{{ formatNumber(totals.done) }}</div>
        <div class="stat-sub">内存中全部任务合计</div>
      </div>
      <div class="stat">
        <div class="stat-label">累计成功</div>
        <div class="stat-value ok">{{ formatNumber(totals.success) }}</div>
      </div>
      <div class="stat">
        <div class="stat-label">累计失败</div>
        <div class="stat-value" :class="totals.failure > 0 ? 'err' : ''">
          {{ formatNumber(totals.failure) }}
        </div>
      </div>
    </section>

    <section class="stack">
      <div class="split">
        <h2 class="section-title">正在压测</h2>
        <div class="split" style="gap: 8px">
          <span v-if="loading" class="spin" />
          <button class="btn btn-sm" @click="refresh">手动刷新</button>
        </div>
      </div>

      <div v-if="running.length === 0" class="card">
        <div class="empty">
          <strong>当前没有正在跑的任务</strong>
          到「新建压测」配置一次请求，提交后这里会实时显示进度。
          <div style="margin-top: 12px">
            <RouterLink to="/new" class="btn btn-primary btn-sm">＋ 新建压测</RouterLink>
          </div>
        </div>
      </div>

      <JobCard
        v-for="job in running"
        :key="job.id"
        :job="job"
        :cancelling="cancellingId === job.id"
        @cancel="handleCancel"
      />
    </section>

    <section class="card">
      <div class="card-head">
        <h2>最近结束的任务</h2>
        <span class="hint">进程重启后内存清空，历史结果请到「历史记录」查看</span>
      </div>

      <div v-if="finished.length === 0" class="empty">还没有已结束的任务</div>

      <div v-else class="table-wrap">
        <table class="table">
          <thead>
            <tr>
              <th>任务</th>
              <th>状态</th>
              <th class="num">请求数</th>
              <th class="num">成功</th>
              <th class="num">失败</th>
              <th class="num">QPS</th>
              <th class="num">耗时</th>
              <th>开始时间</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="job in finished" :key="job.id">
              <td>
                <RouterLink :to="`/jobs/${job.id}`" class="link mono-sm">{{ job.id }}</RouterLink>
              </td>
              <td><span :class="statusClass(job.status)">{{ statusText(job.status) }}</span></td>
              <td class="num">{{ formatNumber(job.progress.total) }}</td>
              <td class="num" style="color: var(--ok)">{{ formatNumber(job.progress.success) }}</td>
              <td class="num" :style="{ color: job.progress.failure > 0 ? 'var(--err)' : undefined }">
                {{ formatNumber(job.progress.failure) }}
              </td>
              <td class="num">{{ formatQps(summaryOf(job)?.qps ?? job.progress.qps) }}</td>
              <td class="num">{{ formatMs(summaryOf(job)?.elapsedMs ?? job.elapsedMs) }}</td>
              <td class="dim mono-sm">{{ formatTime(job.startedAt) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
  </div>
</template>

<style scoped>
.overview {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: 11px;
}

.section-title {
  margin: 0;
  font-size: 13.5px;
  font-weight: 600;
  color: #cfe0f7;
  letter-spacing: 0.3px;
}
</style>
