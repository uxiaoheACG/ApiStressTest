<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import CodeBars from '../components/CodeBars.vue'
import LatencyBars from '../components/LatencyBars.vue'
import RecordDetail from '../components/RecordDetail.vue'
import RecordTable from '../components/RecordTable.vue'
import Sparkline from '../components/Sparkline.vue'
import {
  ApiError,
  cancelJob,
  getJob,
  getJobConfig,
  listHistory,
  listRecords,
} from '../api/client'
import type {
  HistoryEntry,
  JobSnapshot,
  MultipartBody,
  RecordKind,
  RecordLine,
  RecordPage,
  TestConfig,
} from '../api/types'
import { formatBytes, formatMs, formatNumber, formatQps, formatTime, statusClass, statusText } from '../utils/format'

const route = useRoute()
const router = useRouter()
const jobId = computed(() => String(route.params.id ?? ''))

const PAGE_SIZE = 100

const job = ref<JobSnapshot | null>(null)
const historyEntry = ref<HistoryEntry | null>(null)
const loadError = ref('')
const refreshError = ref('')
const cancelling = ref(false)
const cancelHint = ref('')

const qpsSamples = ref<number[]>([])

const kind = ref<RecordKind>('failures')
const page = ref(0)
const records = ref<RecordPage | null>(null)
const recordsLoading = ref(false)
const recordsError = ref('')
const selected = ref<RecordLine | null>(null)

/** 当次压测的完整参数（config_test/<jobID>.json） */
const testConfig = ref<TestConfig | null>(null)
const configLoading = ref(false)
const configError = ref('')

let pollTimer: number | undefined

const running = computed(() => job.value?.status === 'running' || job.value?.status === 'pending')
const summary = computed(() => job.value?.summary ?? historyEntry.value?.summary ?? null)
const latency = computed(() => summary.value?.latency ?? null)
const codes = computed(() => job.value?.progress?.statusCodes ?? summary.value?.statusCodes)
const progressPercent = computed(() => {
  const raw = job.value?.progress?.percent ?? (summary.value ? 100 : 0)
  return Math.max(0, Math.min(100, raw))
})
const offMemory = computed(() => job.value === null && historyEntry.value !== null)
const totalPages = computed(() => {
  const total = records.value?.total ?? 0
  return Math.max(1, Math.ceil(total / PAGE_SIZE))
})

async function fetchSnapshot() {
  try {
    const snap = await getJob(jobId.value)
    job.value = snap
    loadError.value = ''
    refreshError.value = ''

    if (snap.status === 'running' || snap.status === 'pending') {
      const qps = snap.progress?.qps ?? 0
      qpsSamples.value = [...qpsSamples.value, qps].slice(-80)
    } else {
      stopPolling()
      // 任务刚结束：把最终明细拉一次
      if (kind.value === 'failures' && (snap.progress?.failure ?? 0) > 0) {
        await loadRecords()
      }
    }
  } catch (err) {
    if (err instanceof ApiError && err.status === 404) {
      stopPolling()
      await loadFromHistory()
      return
    }
    refreshError.value = err instanceof ApiError ? err.message : (err as Error).message
  }
}

/** 任务不在内存（进程重启过）：退化为从 /history 读取磁盘汇总 */
async function loadFromHistory() {
  if (historyEntry.value) return
  try {
    const res = await listHistory(1000)
    historyEntry.value = res.history.find((item) => item.id === jobId.value) ?? null
    if (!historyEntry.value) {
      loadError.value = `找不到任务 ${jobId.value}：既不在当前进程内存中，也不在 Data 目录里。`
    }
  } catch (err) {
    loadError.value = err instanceof ApiError ? err.message : (err as Error).message
  }
}

async function loadRecords() {
  recordsLoading.value = true
  recordsError.value = ''
  try {
    records.value = await listRecords(jobId.value, kind.value, page.value * PAGE_SIZE, PAGE_SIZE)
  } catch (err) {
    recordsError.value = err instanceof ApiError ? err.message : (err as Error).message
  } finally {
    recordsLoading.value = false
  }
}

/** 读取当次压测的存档参数（config_test/<jobID>.json） */
async function loadConfig() {
  configLoading.value = true
  configError.value = ''
  try {
    const res = await getJobConfig(jobId.value)
    testConfig.value = res.config
  } catch (err) {
    testConfig.value = null
    configError.value =
      err instanceof ApiError && err.status === 404
        ? '这个任务没有参数存档（可能是早期版本产生的任务，或 config_test 目录被清理过）。'
        : `读取参数失败：${err instanceof ApiError ? err.message : (err as Error).message}`
  } finally {
    configLoading.value = false
  }
}

/** 把存档参数带回「新建压测」页，一键复现这次压测 */
function retest() {
  void router.push({ path: '/new', query: { from: jobId.value } })
}

const saveDataText = computed(() => {
  switch (testConfig.value?.saveData) {
    case 0:
      return '0 · 只统计（不写明细）'
    case 1:
      return '1 · 只存失败详情'
    case 2:
      return '2 · 成功简写 + 失败详情'
    case 3:
      return '3 · 全部详情'
    default:
      return '—'
  }
})

const authText = computed(() => {
  const auth = testConfig.value?.authorization
  if (!auth || !auth.type) return '无'
  const token = auth.token ?? ''
  const masked = token.length <= 8 ? '***' : `${token.slice(0, 4)}***`
  return `${auth.type} · ${masked}`
})

const paramRows = computed(() => Object.entries(testConfig.value?.params ?? {}))
const headerRows = computed(() => Object.entries(testConfig.value?.header ?? {}))

interface MultipartSummary {
  fields: Array<{ field: string; value: string; contentType: string }>
  files: Array<{ field: string; filename: string; contentType: string; size: number }>
}

const bodySummary = computed<{
  kind: string
  text: string
  multipart: MultipartSummary | null
}>(() => {
  const body = testConfig.value?.body
  if (!body) return { kind: '—', text: '', multipart: null }

  if (body.type === 'multipart') {
    const data = (body.data ?? {}) as Partial<MultipartBody>
    const fields = Object.entries(data.fields ?? {}).map(([field, value]) => ({
      field,
      value,
      contentType: '',
    }))
    for (const part of data.textParts ?? []) {
      fields.push({ field: part.field, value: part.value, contentType: part.contentType ?? '' })
    }
    return {
      kind: 'multipart/form-data',
      text: '',
      multipart: {
        fields,
        files: (data.files ?? []).map((file) => ({
          field: file.field,
          filename: file.filename,
          contentType: file.contentType,
          size: Math.floor(((file.contentBase64?.length ?? 0) * 3) / 4),
        })),
      },
    }
  }

  const data = body.data
  const text =
    data === null || data === undefined
      ? ''
      : typeof data === 'string'
        ? data
        : JSON.stringify(data, null, 2)
  return { kind: body.type || 'json', text, multipart: null }
})

function startPolling() {
  stopPolling()
  pollTimer = window.setInterval(fetchSnapshot, 1000)
}

function stopPolling() {
  if (pollTimer !== undefined) {
    window.clearInterval(pollTimer)
    pollTimer = undefined
  }
}

async function handleCancel() {
  cancelling.value = true
  cancelHint.value = ''
  try {
    await cancelJob(jobId.value)
    cancelHint.value = '已发送取消信号，等待在飞请求收敛…'
    await fetchSnapshot()
    startPolling()
  } catch (err) {
    refreshError.value = err instanceof ApiError ? err.message : (err as Error).message
  } finally {
    cancelling.value = false
  }
}

function prevPage() {
  if (page.value > 0) page.value -= 1
}

function nextPage() {
  if (page.value + 1 < totalPages.value) page.value += 1
}

function switchKind(next: RecordKind) {
  if (kind.value === next) return
  kind.value = next
  page.value = 0
}

function copyDir() {
  const dir = job.value?.dir ?? historyEntry.value?.dir
  if (!dir) return
  void navigator.clipboard?.writeText(dir)
}

watch([page, kind], () => void loadRecords())

onMounted(async () => {
  await fetchSnapshot()
  await loadRecords()
  await loadConfig()
  if (running.value) startPolling()
})

onUnmounted(stopPolling)
</script>

<template>
  <div class="stack">
    <div v-if="loadError" class="banner banner-err">
      <span>⚠️</span>
      <span>{{ loadError }}</span>
      <div class="spacer" />
      <RouterLink to="/history" class="link">去看历史记录</RouterLink>
    </div>

    <div v-if="refreshError" class="banner banner-err">
      <span>⚠️</span>
      <span>{{ refreshError }}</span>
    </div>

    <div v-if="cancelHint" class="banner banner-info">
      <span>⏹</span>
      <span>{{ cancelHint }}</span>
    </div>

    <div v-if="offMemory" class="banner banner-warn">
      <span>🗄️</span>
      <span>
        该任务已不在当前进程内存中（服务重启过），下面展示的是磁盘上的汇总与明细，无法取消。
      </span>
    </div>

    <!-- 头部信息 -->
    <section class="card">
      <div class="card-head">
        <div class="split" style="gap: 10px">
          <h2>任务 {{ jobId }}</h2>
          <span v-if="job" :class="statusClass(job.status)">{{ statusText(job.status) }}</span>
          <span v-else-if="historyEntry" class="badge">历史</span>
        </div>
        <div class="split" style="gap: 8px">
          <button class="btn btn-sm" @click="fetchSnapshot">刷新</button>
          <button
            v-if="running"
            class="btn btn-danger btn-sm"
            :disabled="cancelling"
            @click="handleCancel"
          >
            {{ cancelling ? '取消中…' : '取消任务' }}
          </button>
          <RouterLink to="/dashboard" class="btn btn-sm">返回监控</RouterLink>
        </div>
      </div>

      <div class="card-body">
        <dl class="kv">
          <dt>开始时间</dt>
          <dd>{{ formatTime(job?.startedAt ?? historyEntry?.startedAt) }}</dd>
          <dt>结果目录</dt>
          <dd>
            {{ job?.dir ?? historyEntry?.dir ?? '—' }}
            <button class="btn btn-ghost btn-sm" title="复制路径" @click="copyDir">复制</button>
          </dd>
          <template v-if="job">
            <dt>已用时间</dt>
            <dd>{{ formatMs(job.elapsedMs) }}</dd>
          </template>
          <template v-if="job?.err">
            <dt>错误</dt>
            <dd style="color: #fca5a5">{{ job.err }}</dd>
          </template>
          <template v-if="historyEntry?.note">
            <dt>磁盘提示</dt>
            <dd style="color: #fde68a">{{ historyEntry.note }}</dd>
          </template>
        </dl>
      </div>
    </section>

    <!-- 当次请求参数（config_test 存档） -->
    <section class="card">
      <div class="card-head">
        <h2>当次请求参数</h2>
        <div class="split" style="gap: 8px">
          <span v-if="configLoading" class="spin" />
          <button class="btn btn-primary btn-sm" :disabled="!testConfig" @click="retest">
            ↻ 重新测试
          </button>
        </div>
      </div>

      <div v-if="configError" class="banner banner-warn" style="margin: 12px 16px">
        <span>ℹ️</span>
        <span>{{ configError }}</span>
      </div>

      <div v-else-if="testConfig" class="card-body stack">
        <dl class="kv">
          <dt>请求</dt>
          <dd>{{ testConfig.method }} {{ testConfig.url }}</dd>
          <dt>并发</dt>
          <dd>
            超时 {{ testConfig.concurrency.timeout }} ms · 并发 {{ testConfig.concurrency.count }} ·
            总请求 {{ formatNumber(testConfig.concurrency.totalRequests) }}
            <span v-if="testConfig.concurrency.rate"> · rate {{ testConfig.concurrency.rate }}</span>
          </dd>
          <dt>结果落盘</dt>
          <dd>{{ saveDataText }}</dd>
          <dt>认证</dt>
          <dd>{{ authText }}</dd>
        </dl>

        <div class="grid-2">
          <div class="field">
            <label>URL 参数（{{ paramRows.length }}）</label>
            <div v-if="paramRows.length" class="chip-list">
              <span v-for="[key, value] in paramRows" :key="key" class="chip">
                {{ key }}={{ value }}
              </span>
            </div>
            <div v-else class="dim" style="font-size: 12px">无</div>
          </div>

          <div class="field">
            <label>请求头（{{ headerRows.length }}）</label>
            <div v-if="headerRows.length" class="chip-list">
              <span v-for="[key, value] in headerRows" :key="key" class="chip">
                {{ key }}: {{ value }}
              </span>
            </div>
            <div v-else class="dim" style="font-size: 12px">无</div>
          </div>
        </div>

        <div class="field">
          <label>请求体（{{ bodySummary.kind }}）</label>

          <template v-if="bodySummary.multipart">
            <div v-if="bodySummary.multipart.fields.length" class="chip-list">
              <span
                v-for="field in bodySummary.multipart.fields"
                :key="field.field"
                class="chip"
              >
                {{ field.field }}={{ field.value }}
                <em v-if="field.contentType" class="chip-extra">{{ field.contentType }}</em>
              </span>
            </div>
            <div v-if="bodySummary.multipart.files.length" class="chip-list">
              <span
                v-for="file in bodySummary.multipart.files"
                :key="`${file.field}-${file.filename}`"
                class="chip"
              >
                📎 {{ file.field }}: {{ file.filename }}（{{ formatBytes(file.size) }}）
              </span>
            </div>
            <div
              v-if="
                !bodySummary.multipart.fields.length && !bodySummary.multipart.files.length
              "
              class="dim"
              style="font-size: 12px"
            >
              无内容
            </div>
          </template>

          <pre v-else-if="bodySummary.text" class="block">{{ bodySummary.text }}</pre>
          <div v-else class="dim" style="font-size: 12px">无请求体</div>
        </div>

        <div class="dim" style="font-size: 11.5px">
          参数来自 config_test/{{ jobId }}.json；点「重新测试」会带着这些参数跳到新建压测页。
        </div>
      </div>

      <div v-else-if="!configLoading" class="empty">暂无参数存档</div>
    </section>

    <!-- 实时进度 -->
    <section v-if="job" class="card">
      <div class="card-head">
        <h2>实时进度</h2>
        <span class="hint">{{ running ? '每秒自动刷新' : '任务已结束' }}</span>
      </div>
      <div class="card-body stack">
        <div>
          <div class="split" style="margin-bottom: 7px">
            <span class="muted" style="font-size: 12.5px">
              完成 <strong>{{ formatNumber(job.progress.done) }}</strong> /
              {{ formatNumber(job.progress.total) }}
            </span>
            <span style="color: var(--accent); font-weight: 600">
              {{ progressPercent.toFixed(1) }}%
            </span>
          </div>
          <div class="progress">
            <div
              class="progress-bar"
              :class="{ running }"
              :style="{ width: `${progressPercent}%` }"
            />
          </div>
        </div>

        <div class="grid-3">
          <div class="stat">
            <div class="stat-label">成功</div>
            <div class="stat-value ok">{{ formatNumber(job.progress.success) }}</div>
          </div>
          <div class="stat">
            <div class="stat-label">失败</div>
            <div class="stat-value" :class="job.progress.failure > 0 ? 'err' : ''">
              {{ formatNumber(job.progress.failure) }}
            </div>
            <div class="stat-sub">
              超时 {{ formatNumber(job.progress.timeout) }} · 错误
              {{ formatNumber(job.progress.error) }} · 非 2xx
              {{ formatNumber(job.progress.unsuccess) }}
            </div>
          </div>
          <div class="stat">
            <div class="stat-label">实时 QPS</div>
            <div class="stat-value accent">{{ formatQps(job.progress.qps) }}</div>
          </div>
        </div>

        <div v-if="running" class="trend">
          <div class="split">
            <span class="muted" style="font-size: 12px">QPS 趋势（最近 80 次采样）</span>
          </div>
          <Sparkline :points="qpsSamples" />
        </div>

        <div v-if="codes && Object.keys(codes).length">
          <div class="muted" style="font-size: 12px; margin-bottom: 8px">状态码分布</div>
          <CodeBars :codes="codes" :total="job.progress.done" />
        </div>
      </div>
    </section>

    <!-- 汇总 -->
    <section v-if="summary" class="card">
      <div class="card-head">
        <h2>汇总结果</h2>
        <span class="hint">与任务目录下的 summary.json 一致</span>
      </div>
      <div class="card-body stack">
        <div class="grid-3">
          <div class="stat">
            <div class="stat-label">总请求</div>
            <div class="stat-value">{{ formatNumber(summary.total) }}</div>
          </div>
          <div class="stat">
            <div class="stat-label">成功</div>
            <div class="stat-value ok">{{ formatNumber(summary.success) }}</div>
          </div>
          <div class="stat">
            <div class="stat-label">失败</div>
            <div class="stat-value" :class="summary.failure > 0 ? 'err' : ''">
              {{ formatNumber(summary.failure) }}
            </div>
            <div class="stat-sub">
              超时 {{ formatNumber(summary.timeout) }} · 错误
              {{ formatNumber(summary.error) }} · 非 2xx {{ formatNumber(summary.unsuccess) }}
            </div>
          </div>
          <div class="stat">
            <div class="stat-label">平均 QPS</div>
            <div class="stat-value accent">{{ formatQps(summary.qps) }}</div>
          </div>
          <div class="stat">
            <div class="stat-label">总耗时</div>
            <div class="stat-value">{{ formatMs(summary.elapsedMs) }}</div>
          </div>
          <div class="stat">
            <div class="stat-label">成功率</div>
            <div class="stat-value" :class="summary.failure > 0 ? 'warn' : 'ok'">
              {{ summary.total > 0 ? ((summary.success / summary.total) * 100).toFixed(2) : '0' }}%
            </div>
          </div>
        </div>

        <div v-if="latency" class="grid-2">
          <div>
            <div class="muted" style="font-size: 12px; margin-bottom: 8px">响应耗时分布</div>
            <LatencyBars :latency="latency" />
          </div>
          <div>
            <div class="muted" style="font-size: 12px; margin-bottom: 8px">状态码分布</div>
            <CodeBars :codes="summary.statusCodes" :total="summary.total" />
          </div>
        </div>
      </div>
    </section>

    <!-- 明细 -->
    <section class="card">
      <div class="card-head">
        <h2>结果明细</h2>
        <div class="split" style="gap: 8px">
          <div class="tabs">
            <button
              class="tab"
              :class="{ active: kind === 'failures' }"
              @click="switchKind('failures')"
            >
              失败 / 超时
            </button>
            <button
              class="tab"
              :class="{ active: kind === 'success' }"
              @click="switchKind('success')"
            >
              成功
            </button>
          </div>
          <button class="btn btn-sm" :disabled="recordsLoading" @click="loadRecords">刷新</button>
        </div>
      </div>

      <div v-if="recordsError" class="banner banner-err" style="margin: 12px 16px">
        <span>⚠️</span><span>{{ recordsError }}</span>
      </div>

      <div v-if="records?.note" class="banner banner-warn" style="margin: 12px 16px">
        <span>ℹ️</span><span>{{ records.note }}</span>
      </div>

      <RecordTable :lines="records?.lines ?? []" :kind="kind" :loading="recordsLoading" @select="selected = $event" />

      <div class="pager">
        <button class="btn btn-sm" :disabled="page === 0" @click="prevPage">← 上一页</button>
        <span class="dim" style="font-size: 12px">
          第 {{ page + 1 }} / {{ totalPages }} 页 · 共
          {{ formatNumber(records?.total ?? 0) }} 条
        </span>
        <button class="btn btn-sm" :disabled="page + 1 >= totalPages" @click="nextPage">
          下一页 →
        </button>
        <div class="spacer" />
        <span class="dim" style="font-size: 11.5px">
          {{ kind === 'failures' ? 'failures.jsonl' : 'success.jsonl' }} · 点击任意一行查看完整请求/响应
        </span>
      </div>
    </section>

    <RecordDetail :line="selected" @close="selected = null" />
  </div>
</template>

<style scoped>
.trend {
  border-top: 1px solid var(--border);
  padding-top: 11px;
}

.pager {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 11px 16px;
  border-top: 1px solid var(--border);
  flex-wrap: wrap;
}

.chip-list {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.chip-extra {
  font-style: normal;
  color: var(--accent);
  margin-left: 4px;
  font-size: 10.5px;
}
</style>
