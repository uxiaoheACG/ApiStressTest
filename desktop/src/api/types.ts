/** 与 Go 后端 model.TestConfig / job.Snapshot 对应的类型定义 */

/** 任务状态，对应 job.Status.String() */
export type JobStatus = 'pending' | 'running' | 'completed' | 'failed' | 'canceled' | 'unknown'

/** 状态码分布：key 是状态码，value 是次数 */
export type StatusCodes = Record<string, number>

/** 压测配置，POST /getData 的请求体 */
export interface TestConfig {
  url: string
  method: string
  body: {
    type: 'json' | 'form' | 'text' | 'raw' | 'multipart' | ''
    data: unknown
  }
  header: Record<string, string>
  params: Record<string, string>
  authorization: {
    type: '' | 'bearer' | 'basic' | 'apikey'
    token: string
  }
  concurrency: {
    /** 单请求超时，单位毫秒 */
    timeout: number
    /** 最大同时并发 */
    count: number
    /** 总请求数 */
    totalRequests: number
    /** 每秒发送请求数（后端暂未生效） */
    rate: number
  }
  /** 业务码字段路径（如 "code"、"error.code"）；留空自动探测常见字段 */
  bizCodeField?: string
}

/** multipart 里的一个文件字段，内容以 base64 传递 */
export interface MultipartFilePayload {
  field: string
  filename: string
  contentType: string
  contentBase64: string
}

/** multipart 的文本字段（可单独指定该部分的 Content-Type） */
export interface MultipartTextPart {
  field: string
  value: string
  contentType?: string
}

/** body.type=multipart 时的 data 结构 */
export interface MultipartBody {
  fields: Record<string, string>
  textParts?: MultipartTextPart[]
  files: MultipartFilePayload[]
}

/** 前端 multipart 文本字段的编辑行（contentType 允许为空字符串） */
export interface TextFieldRow {
  field: string
  value: string
  contentType: string
}

/** 实时进度 */
export interface Progress {
  done: number
  total: number
  percent: number
  success: number
  failure: number
  timeout: number
  error: number
  unsuccess: number
  qps: number
  statusCodes?: StatusCodes
}

/** 耗时统计，单位毫秒 */
export interface Latency {
  min: number
  p50: number
  p95: number
  p99: number
  max: number
}

/** 任务汇总结果，与 summary.json 一致 */
export interface Summary {
  jobId: string
  total: number
  success: number
  failure: number
  timeout: number
  error: number
  unsuccess: number
  statusCodes?: StatusCodes
  qps: number
  elapsedMs: number
  latency: Latency
}

/** GET /jobs/:id 的响应体 */
export interface JobSnapshot {
  id: string
  status: JobStatus
  dir: string
  startedAt: string
  elapsedMs: number
  progress: Progress
  err?: string
  summary?: Summary
}

/** GET /history 的元素 */
export interface HistoryEntry {
  id: string
  dir: string
  startedAt: string
  summary?: Summary
  note?: string
}

/** POST /getData 的响应体 */
export interface StartResult {
  code: number
  jobId: string
  status: JobStatus
  total: number
  dir: string
}

/** POST 启动成功后的任务卡片数据来源（JobSnapshot 的超集） */
export interface StartedJob {
  code: number
  jobId: string
  status: JobStatus
  total: number
  dir: string
}

/** 旧版本任务 success.jsonl 里的一行：saveData=2 时是简写，3 时是详细结构 */
export interface SuccessRecordLine {
  i: number
  s: number
  ms: number
  size: number
  at: string
}

/** failures.jsonl 里的一行（saveData=3 时成功记录也是这个结构） */
export interface FailureRecordLine {
  i: number
  kind: string
  errKind?: string
  err?: string
  method: string
  url: string
  at: string
  ms: number
  reqHeader?: Record<string, string>
  reqBody?: string
  reqBodyTruncated?: boolean
  status: number
  respHeader?: Record<string, string>
  respBody?: string
  respBodyTruncated?: boolean
  respBytes: number
}

/** 明细行：两种记录结构按需读取字段 */
export type RecordLine = Partial<SuccessRecordLine & FailureRecordLine> & { i: number; at: string }

/** GET /jobs/:id/records 的响应体 */
export interface RecordPage {
  code: number
  jobId: string
  dir: string
  kind: 'success' | 'failures'
  file: string
  offset: number
  limit: number
  total: number
  lines: RecordLine[]
  note?: string
}

/** 明细类型 */
export type RecordKind = 'success' | 'failures'

/** 一种返回类型的聚合统计（result.db 里的一行） */
export interface AnalysisTypeStat {
  fingerprint: string
  statusCode: number
  bizCode: string
  kind: string
  errKind: string
  count: number
  firstSeenAt: string
  lastSeenAt: string
  sampleFile: string
  reqMethod: string
  reqUrl: string
}

/** GET /jobs/:id/analysis 里 report 字段 */
export interface AnalysisReport {
  jobId: string
  total: number
  done: number
  canceled: boolean
  createdAt: string
  finishedAt?: string
  typeCount: number
  types: AnalysisTypeStat[]
}

/** 样本文件内容（每种返回类型的第一份完整快照） */
export interface SampleFile {
  fingerprint: string
  kind: string
  status: number
  bizCode?: string
  errKind?: string
  err?: string
  method: string
  url: string
  at: string
  ms: number
  reqHeader?: Record<string, string>
  reqBody?: string
  reqBodyTruncated?: boolean
  respHeader?: Record<string, string>
  respBody?: string
  respBodyTruncated?: boolean
  respBytes: number
}
