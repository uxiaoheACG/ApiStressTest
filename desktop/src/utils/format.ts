/** 展示层的格式化工具，全部纯函数，方便在模板里直接用 */

/** 毫秒 → 人类可读耗时 */
export function formatMs(ms: number | undefined | null): string {
  if (ms === undefined || ms === null || Number.isNaN(ms)) return '—'
  if (ms < 1000) return `${Math.round(ms)} ms`
  if (ms < 60_000) return `${(ms / 1000).toFixed(2)} s`
  const minutes = Math.floor(ms / 60_000)
  const seconds = ((ms % 60_000) / 1000).toFixed(0)
  return `${minutes} min ${seconds} s`
}

/** 数字 → 千分位 */
export function formatNumber(n: number | undefined | null): string {
  if (n === undefined || n === null || Number.isNaN(n)) return '—'
  return n.toLocaleString('zh-CN', { maximumFractionDigits: 0 })
}

/** QPS 等浮点数 */
export function formatQps(n: number | undefined | null): string {
  if (n === undefined || n === null || Number.isNaN(n)) return '—'
  if (n >= 1000) return n.toLocaleString('zh-CN', { maximumFractionDigits: 0 })
  return n.toFixed(1)
}

/** 百分比 */
export function formatPercent(n: number | undefined | null): string {
  if (n === undefined || n === null || Number.isNaN(n)) return '—'
  return `${n.toFixed(n >= 10 ? 0 : 1)}%`
}

/** 字节数 */
export function formatBytes(bytes: number | undefined | null): string {
  if (bytes === undefined || bytes === null || Number.isNaN(bytes)) return '—'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(2)} MB`
}

/** RFC3339 时间 → 本地时间字符串 */
export function formatTime(iso: string | undefined | null): string {
  if (!iso) return '—'
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return iso
  const pad = (n: number) => String(n).padStart(2, '0')
  return (
    `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ` +
    `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
  )
}

/** 只保留时分秒，用于趋势轴 */
export function formatClock(iso: string | undefined | null): string {
  const text = formatTime(iso)
  return text === '—' ? text : text.slice(11)
}

/** 任务状态 → 中文文案 */
export function statusText(status: string | undefined): string {
  switch (status) {
    case 'pending':
      return '排队中'
    case 'running':
      return '压测中'
    case 'completed':
      return '已完成'
    case 'failed':
      return '失败'
    case 'canceled':
      return '已取消'
    default:
      return '未知'
  }
}

/** 任务状态 → 徽标样式类 */
export function statusClass(status: string | undefined): string {
  switch (status) {
    case 'running':
      return 'badge badge-running'
    case 'pending':
      return 'badge badge-pending'
    case 'completed':
      return 'badge badge-ok'
    case 'failed':
      return 'badge badge-err'
    case 'canceled':
      return 'badge badge-warn'
    default:
      return 'badge'
  }
}

/** 失败类型 → 中文文案 */
export function failureKindText(kind: string | undefined): string {
  switch (kind) {
    case 'timeout':
      return '超时'
    case 'unsuccess':
      return '非 2xx'
    case 'error':
      return '网络错误'
    case 'success':
      return '成功'
    default:
      return kind || '—'
  }
}

/** 错误归类 → 中文文案 */
export function errKindText(errKind: string | undefined): string {
  switch (errKind) {
    case 'deadline':
      return '超时/期限'
    case 'canceled':
      return '已取消'
    case 'dns':
      return 'DNS 解析'
    case 'tls':
      return 'TLS/证书'
    case 'eof':
      return '连接中断'
    case 'refused':
      return '连接被拒绝'
    case 'reset':
      return '连接被重置'
    case 'other':
      return '其它'
    default:
      return errKind || '—'
  }
}

/** 状态码 → 配色类（2xx 绿 / 3xx 蓝 / 4xx 橙 / 5xx 红） */
export function codeClass(code: number | undefined): string {
  if (!code) return 'code code-none'
  if (code >= 200 && code < 300) return 'code code-ok'
  if (code >= 300 && code < 400) return 'code code-info'
  if (code >= 400 && code < 500) return 'code code-warn'
  if (code >= 500) return 'code code-err'
  return 'code'
}

/** 状态码分布 map → 按次数从多到少排序的数组 */
export function sortedCodes(codes: Record<string, number> | undefined): Array<[string, number]> {
  if (!codes) return []
  return Object.entries(codes).sort((a, b) => b[1] - a[1])
}
