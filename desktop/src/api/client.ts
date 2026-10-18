import type {
  HistoryEntry,
  JobSnapshot,
  RecordKind,
  RecordPage,
  StartResult,
  TestConfig,
} from './types'

const BASE_URL_KEY = 'ast.baseUrl'
const DEFAULT_BASE_URL = 'http://127.0.0.1:3010'
/** 旧版本的默认后端地址：没手动改过时跟随新默认端口 */
const LEGACY_DEFAULT_BASE_URL = 'http://127.0.0.1:8080'

/** 后端地址：默认本机 3010，可在设置页修改并持久化 */
export function getBaseUrl(): string {
  const saved = localStorage.getItem(BASE_URL_KEY)
  if (!saved || saved === LEGACY_DEFAULT_BASE_URL) return DEFAULT_BASE_URL
  return saved
}

export function setBaseUrl(url: string): void {
  const trimmed = url.trim().replace(/\/+$/, '')
  if (trimmed) {
    localStorage.setItem(BASE_URL_KEY, trimmed)
  } else {
    localStorage.removeItem(BASE_URL_KEY)
  }
}

export function resetBaseUrl(): void {
  localStorage.removeItem(BASE_URL_KEY)
}

/** 后端返回的非 2xx 响应 */
export class ApiError extends Error {
  status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

interface RawResponse {
  status: number
  body: string
}

/**
 * 发一次请求。
 *
 * 在 Electron 里走 preload 暴露的主进程通道（Node 侧 fetch），因此不受页面
 * file:// 来源的 CORS / 私有网络策略限制；在浏览器里打开时退回 window.fetch
 * （后端已通过 CORS 放行本机来源，两种方式都能用）。
 */
async function rawRequest(method: string, path: string, body?: unknown): Promise<RawResponse> {
  const url = getBaseUrl() + path

  if (window.ast) {
    const res = await window.ast.request({ method, url, body })
    return { status: res.status, body: res.body }
  }

  const res = await fetch(url, {
    method,
    headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  return { status: res.status, body: await res.text() }
}

/** 发请求并解析 JSON；非 2xx 时抛出 ApiError，消息取后端的 error 字段 */
async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  let res: RawResponse
  try {
    res = await rawRequest(method, path, body)
  } catch (err) {
    throw new ApiError(0, `无法连接后端 ${getBaseUrl()}：${(err as Error).message}`)
  }

  const text = res.body
  let data: unknown = null
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      throw new ApiError(res.status, `响应不是合法 JSON（HTTP ${res.status}）`)
    }
  }

  if (res.status < 200 || res.status >= 300) {
    const message =
      (data as { error?: string } | null)?.error || `请求失败（HTTP ${res.status}）`
    throw new ApiError(res.status, message)
  }

  return data as T
}

/** 健康检查 */
export function health(): Promise<{ status: string }> {
  return request('GET', '/')
}

/** 启动压测任务，返回 jobId */
export function startTest(config: TestConfig): Promise<StartResult> {
  return request('POST', '/getData', config)
}

/** 内存中的任务列表（新→旧） */
export function listJobs(): Promise<{ code: number; jobs: JobSnapshot[] }> {
  return request('GET', '/jobs')
}

/** 单个任务快照 */
export function getJob(id: string): Promise<JobSnapshot> {
  return request('GET', `/jobs/${encodeURIComponent(id)}`)
}

/** 取消任务（协作式，202 表示信号已发出） */
export function cancelJob(id: string): Promise<{ code: number; jobId: string; status: string }> {
  return request('DELETE', `/jobs/${encodeURIComponent(id)}`)
}

/** 历史任务（磁盘扫描，重启后仍在） */
export function listHistory(limit = 0): Promise<{
  code: number
  count: number
  history: HistoryEntry[]
}> {
  return request('GET', `/history?limit=${limit}`)
}

/** 结果明细分页 */
export function listRecords(
  id: string,
  kind: RecordKind,
  offset: number,
  limit: number,
): Promise<RecordPage> {
  const query = new URLSearchParams({
    kind,
    offset: String(offset),
    limit: String(limit),
  })
  return request('GET', `/jobs/${encodeURIComponent(id)}/records?${query.toString()}`)
}

/** 当次压测的完整参数（config_test/<jobID>.json），用于历史详情与重新测试 */
export function getJobConfig(
  id: string,
): Promise<{ code: number; jobId: string; config: TestConfig }> {
  return request('GET', `/jobs/${encodeURIComponent(id)}/config`)
}
