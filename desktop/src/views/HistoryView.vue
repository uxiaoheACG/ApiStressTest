<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ApiError, listHistory } from '../api/client'
import type { HistoryEntry } from '../api/types'
import { formatMs, formatNumber, formatQps, formatTime } from '../utils/format'

const router = useRouter()

const entries = ref<HistoryEntry[]>([])
const loading = ref(false)
const errorMsg = ref('')
const keyword = ref('')

const filtered = computed(() => {
  const key = keyword.value.trim().toLowerCase()
  if (!key) return entries.value
  return entries.value.filter(
    (item) => item.id.toLowerCase().includes(key) || item.dir.toLowerCase().includes(key),
  )
})

const totals = computed(() => {
  let requests = 0
  let success = 0
  let failure = 0
  for (const item of entries.value) {
    requests += item.summary?.total ?? 0
    success += item.summary?.success ?? 0
    failure += item.summary?.failure ?? 0
  }
  return { requests, success, failure }
})

async function load() {
  loading.value = true
  errorMsg.value = ''
  try {
    const res = await listHistory(0)
    entries.value = res.history ?? []
  } catch (err) {
    errorMsg.value = err instanceof ApiError ? err.message : (err as Error).message
  } finally {
    loading.value = false
  }
}

function open(item: HistoryEntry) {
  void router.push(`/jobs/${item.id}`)
}

onMounted(load)
</script>

<template>
  <div class="stack">
    <div v-if="errorMsg" class="banner banner-err">
      <span>⚠️</span>
      <span>{{ errorMsg }}</span>
      <div class="spacer" />
      <button class="btn btn-sm" @click="load">重试</button>
    </div>

    <section class="overview">
      <div class="stat">
        <div class="stat-label">历史任务数</div>
        <div class="stat-value accent">{{ formatNumber(entries.length) }}</div>
        <div class="stat-sub">扫描 Data 目录得到</div>
      </div>
      <div class="stat">
        <div class="stat-label">累计请求</div>
        <div class="stat-value">{{ formatNumber(totals.requests) }}</div>
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

    <section class="card">
      <div class="card-head">
        <h2>历史压测结果</h2>
        <div class="split" style="gap: 8px">
          <input
            v-model="keyword"
            class="input"
            style="width: 210px; padding: 5px 10px; font-size: 12.5px"
            placeholder="按任务 ID / 目录筛选"
          />
          <button class="btn btn-sm" :disabled="loading" @click="load">
            {{ loading ? '刷新中…' : '刷新' }}
          </button>
        </div>
      </div>

      <div v-if="!loading && filtered.length === 0" class="empty">
        <strong>没有历史记录</strong>
        跑一次压测后，结果会写入 Data 目录，重启服务也依然能在这里查到。
      </div>

      <div v-else class="table-wrap">
        <table class="table">
          <thead>
            <tr>
              <th>开始时间</th>
              <th>任务 ID</th>
              <th class="num">总请求</th>
              <th class="num">成功</th>
              <th class="num">失败</th>
              <th class="num">QPS</th>
              <th class="num">P95</th>
              <th class="num">耗时</th>
              <th>备注</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in filtered" :key="item.id" @click="open(item)">
              <td class="mono-sm">{{ formatTime(item.startedAt) }}</td>
              <td class="mono-sm">{{ item.id }}</td>
              <td class="num">{{ formatNumber(item.summary?.total) }}</td>
              <td class="num" style="color: var(--ok)">{{ formatNumber(item.summary?.success) }}</td>
              <td
                class="num"
                :style="{ color: (item.summary?.failure ?? 0) > 0 ? 'var(--err)' : undefined }"
              >
                {{ formatNumber(item.summary?.failure) }}
              </td>
              <td class="num">{{ formatQps(item.summary?.qps) }}</td>
              <td class="num mono-sm">{{ formatMs(item.summary?.latency.p95) }}</td>
              <td class="num mono-sm">{{ formatMs(item.summary?.elapsedMs) }}</td>
              <td>
                <span v-if="item.note" class="badge badge-warn">{{ item.note }}</span>
                <span v-else class="dim mono-sm">—</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <div class="dim" style="font-size: 11.5px">
      提示：历史记录不含实时进度，也无法取消；点击任意一行可查看该任务的汇总与明细。
    </div>
  </div>
</template>

<style scoped>
.overview {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: 11px;
}
</style>
