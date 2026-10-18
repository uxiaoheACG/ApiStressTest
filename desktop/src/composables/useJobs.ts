import { computed, onUnmounted, ref } from 'vue'
import { ApiError, listJobs } from '../api/client'
import type { JobSnapshot } from '../api/types'

/**
 * 任务列表轮询（模块级单例）。
 *
 * 运行监控页与新建压测页共用同一份数据：只要有一处 subscribe，就有一个
 * 1 秒定时器在刷新 GET /jobs，页面切换时不会重复请求。
 */
const jobs = ref<JobSnapshot[]>([])
const loading = ref(false)
const error = ref('')
const lastUpdated = ref<number>(0)

let timer: number | undefined
let subscribers = 0

async function refresh(): Promise<void> {
  loading.value = true
  try {
    const res = await listJobs()
    jobs.value = res.jobs ?? []
    error.value = ''
    lastUpdated.value = Date.now()
  } catch (err) {
    error.value = err instanceof ApiError ? err.message : (err as Error).message
  } finally {
    loading.value = false
  }
}

function startLoop(): void {
  if (timer !== undefined) return
  void refresh()
  timer = window.setInterval(refresh, 1000)
}

function stopLoop(): void {
  if (timer !== undefined) {
    window.clearInterval(timer)
    timer = undefined
  }
}

/** 在组件生命周期内使用：自动启动/停止轮询 */
export function useJobPolling() {
  subscribers += 1
  startLoop()

  onUnmounted(() => {
    subscribers -= 1
    if (subscribers <= 0) {
      subscribers = 0
      stopLoop()
    }
  })

  return { jobs, loading, error, refresh, lastUpdated, stopLoop, startLoop }
}

/** 运行中的任务（pending / running），新的在前 */
export function activeJobs(list: JobSnapshot[]): JobSnapshot[] {
  return list.filter((j) => j.status === 'running' || j.status === 'pending')
}

/** 已结束的任务 */
export function finishedJobs(list: JobSnapshot[]): JobSnapshot[] {
  return list.filter((j) => j.status !== 'running' && j.status !== 'pending')
}

/** 用任务进度算一个稳定的进度百分比（后端已给 percent，这里兜底 0-100） */
export function progressPercent(job: JobSnapshot | undefined): number {
  if (!job) return 0
  const raw = job.progress?.percent ?? 0
  if (!Number.isFinite(raw)) return 0
  return Math.max(0, Math.min(100, raw))
}

/** 失败 + 超时 + 错误 + 非 2xx 的数量 */
export function failureTotal(job: JobSnapshot | undefined): number {
  if (!job) return 0
  const p = job.progress
  return (p?.failure ?? 0) + (p?.timeout ?? 0) + (p?.error ?? 0) + (p?.unsuccess ?? 0)
}

/** 最近一次刷新距今秒数，用于顶部提示 */
export function useFreshness() {
  return computed(() => {
    if (!lastUpdated.value) return '—'
    const seconds = Math.max(0, Math.round((Date.now() - lastUpdated.value) / 1000))
    return seconds <= 1 ? '刚刚' : `${seconds} 秒前`
  })
}
