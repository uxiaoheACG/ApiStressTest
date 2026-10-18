<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import KeyValueEditor from '../components/KeyValueEditor.vue'
import MultipartFieldsEditor from '../components/MultipartFieldsEditor.vue'
import { ApiError, getJobConfig, startTest } from '../api/client'
import type { MultipartBody, MultipartFilePayload, TestConfig, TextFieldRow } from '../api/types'
import { activeJobs, useJobPolling } from '../composables/useJobs'
import { formatBytes, formatNumber } from '../utils/format'

const route = useRoute()
const router = useRouter()
const { jobs } = useJobPolling()

const LAST_CONFIG_KEY = 'ast.lastConfig'
/** 单个上传文件的大小上限，与后端 16 MB 的限制留出余量 */
const MAX_FILE_BYTES = 10 * 1024 * 1024

type BodyType = 'json' | 'form' | 'multipart' | 'text' | 'raw'

/** multipart 文件行：在载荷之外额外记录原始大小用于展示 */
interface FileRow extends MultipartFilePayload {
  size: number
}

const url = ref('http://127.0.0.1:3010/')
const method = ref('GET')
const saveData = ref<0 | 1 | 2 | 3>(2)

const bodyType = ref<BodyType>('json')
const jsonText = ref('{\n  "hello": "world"\n}')
const rawText = ref('')
const formData = ref<Record<string, string>>({})
const multipartFields = ref<TextFieldRow[]>([])
const multipartFiles = ref<FileRow[]>([])
const pendingFileField = ref('file')
const fileInput = ref<HTMLInputElement | null>(null)

const header = ref<Record<string, string>>({ Accept: 'application/json' })
const params = ref<Record<string, string>>({})

const authType = ref<'' | 'bearer' | 'basic' | 'apikey'>('')
const authToken = ref('')

const timeout = ref(3000)
const count = ref(50)
const totalRequests = ref(1000)
const rate = ref(100)

const submitting = ref(false)
const errorMsg = ref('')
const notice = ref('')
/** 从哪个历史任务回填的配置（用于提示） */
const sourceJob = ref('')

const runningCount = computed(() => activeJobs(jobs.value).length)

const saveDataOptions = [
  { value: 0, label: '0 · 只统计', desc: '只生成 summary.json，不写明细' },
  { value: 1, label: '1 · 只存失败', desc: '仅写 failures.jsonl 详细记录' },
  { value: 2, label: '2 · 成功简写 + 失败详情', desc: '推荐：成功记一行简写，失败记详情' },
  { value: 3, label: '3 · 全部详情', desc: '成功与失败都写请求/响应详情' },
] as const

const bodyTypes: Array<{ value: BodyType; label: string; hint: string }> = [
  { value: 'json', label: 'json', hint: 'application/json，保留字符串/数字/布尔类型' },
  { value: 'form', label: 'form', hint: 'application/x-www-form-urlencoded，值一律按字符串发送' },
  { value: 'multipart', label: 'multipart', hint: 'multipart/form-data，可上传文件' },
  { value: 'text', label: 'text', hint: 'text/plain' },
  { value: 'raw', label: 'raw', hint: 'text/plain（原样发送）' },
]

const currentBodyHint = computed(
  () => bodyTypes.find((item) => item.value === bodyType.value)?.hint ?? '',
)

const methods = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS']

/** JSON 编辑器的实时解析结果：合法性 + 顶层字段类型，帮用户确认类型没丢 */
const jsonAnalysis = computed(() => {
  const text = jsonText.value.trim()
  if (!text) {
    return { ok: true, message: '空请求体：不会发送 body', fields: [] as TypeRow[] }
  }
  let parsed: unknown
  try {
    parsed = JSON.parse(text)
  } catch (err) {
    return { ok: false, message: `✗ ${(err as Error).message}`, fields: [] as TypeRow[] }
  }
  if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed)) {
    return { ok: true, message: `✓ 合法 JSON，顶层类型 ${typeName(parsed)}`, fields: [] as TypeRow[] }
  }
  const fields = Object.entries(parsed as Record<string, unknown>).map(([key, value]) => ({
    key,
    type: typeName(value),
    preview: previewOf(value),
  }))
  return { ok: true, message: `✓ 合法 JSON，${fields.length} 个顶层字段`, fields }
})

interface TypeRow {
  key: string
  type: string
  preview: string
}

function typeName(value: unknown): string {
  if (value === null) return 'null'
  if (Array.isArray(value)) return 'array'
  return typeof value
}

function previewOf(value: unknown): string {
  const text = typeof value === 'string' ? `"${value}"` : JSON.stringify(value) ?? String(value)
  return text.length > 42 ? `${text.slice(0, 42)}…` : text
}

function pickFile() {
  fileInput.value?.click()
}

function readAsBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => {
      const result = String(reader.result ?? '')
      const comma = result.indexOf(',')
      resolve(comma >= 0 ? result.slice(comma + 1) : result)
    }
    reader.onerror = () => reject(reader.error ?? new Error('读取文件失败'))
    reader.readAsDataURL(file)
  })
}

async function onFileChange(event: Event) {
  const input = event.target as HTMLInputElement
  const picked = Array.from(input.files ?? [])
  for (const file of picked) {
    if (file.size > MAX_FILE_BYTES) {
      errorMsg.value = `文件「${file.name}」${formatBytes(file.size)}，超过 10 MB 上限`
      continue
    }
    try {
      const contentBase64 = await readAsBase64(file)
      multipartFiles.value.push({
        field: pendingFileField.value.trim() || 'file',
        filename: file.name,
        contentType: file.type || 'application/octet-stream',
        contentBase64,
        size: file.size,
      })
      errorMsg.value = ''
    } catch (err) {
      errorMsg.value = `读取文件「${file.name}」失败：${(err as Error).message}`
    }
  }
  // 清空以便重复选择同一个文件
  input.value = ''
}

function removeFile(index: number) {
  multipartFiles.value.splice(index, 1)
}

/** 校验并组装后端需要的 TestConfig */
function buildConfig(): TestConfig {
  const trimmedUrl = url.value.trim()
  if (!trimmedUrl) throw new Error('请填写请求地址')
  if (!/^https?:\/\//i.test(trimmedUrl)) throw new Error('请求地址需以 http:// 或 https:// 开头')
  if (!Number.isFinite(timeout.value) || timeout.value < 1) throw new Error('超时时间需 ≥ 1 ms')
  if (!Number.isFinite(count.value) || count.value < 1) throw new Error('并发数需 ≥ 1')
  if (!Number.isFinite(totalRequests.value) || totalRequests.value < 1) {
    throw new Error('总请求数需 ≥ 1')
  }

  let bodyData: unknown = null

  if (bodyType.value === 'json') {
    const text = jsonText.value.trim()
    if (text) {
      try {
        bodyData = JSON.parse(text)
      } catch {
        throw new Error('请求体不是合法 JSON')
      }
    }
  } else if (bodyType.value === 'form') {
    bodyData = { ...formData.value }
  } else if (bodyType.value === 'multipart') {
    const rows = multipartFields.value.filter((row) => row.field.trim())
    const body: MultipartBody = {
      fields: {},
      textParts: rows.map((row) => ({
        field: row.field.trim(),
        value: row.value,
        contentType: row.contentType.trim() || undefined,
      })),
      files: multipartFiles.value.map(({ field, filename, contentType, contentBase64 }) => ({
        field,
        filename,
        contentType,
        contentBase64,
      })),
    }
    if ((body.textParts?.length ?? 0) === 0 && body.files.length === 0) {
      throw new Error('multipart 请求体至少要有一个字段或一个文件')
    }
    bodyData = body
  } else {
    bodyData = rawText.value
  }

  return {
    url: trimmedUrl,
    method: method.value,
    saveData: saveData.value,
    body: { type: bodyType.value, data: bodyData },
    header: { ...header.value },
    params: { ...params.value },
    authorization: { type: authType.value, token: authToken.value },
    concurrency: {
      timeout: Math.round(timeout.value),
      count: Math.round(count.value),
      totalRequests: Math.round(totalRequests.value),
      rate: Math.max(0, Math.round(rate.value || 0)),
    },
  }
}

/** 记住本次参数（不含文件内容，避免撑爆 localStorage） */
function rememberConfig(config: TestConfig) {
  try {
    const slim = JSON.parse(JSON.stringify(config)) as TestConfig
    if (slim.body?.type === 'multipart') {
      const body = slim.body.data as MultipartBody
      body.files = (body.files ?? []).map((file) => ({ ...file, contentBase64: '' }))
    }
    localStorage.setItem(LAST_CONFIG_KEY, JSON.stringify(slim))
  } catch {
    // localStorage 写满等情况忽略，不影响提交
  }
}

/** 把一份配置填进表单（来自上次配置或历史任务） */
function applyConfig(conf: TestConfig) {
  url.value = conf.url ?? ''
  method.value = conf.method || 'GET'
  saveData.value = (conf.saveData ?? 2) as 0 | 1 | 2 | 3

  const type = (conf.body?.type || 'json') as BodyType
  bodyType.value = type
  const data = conf.body?.data

  if (type === 'json') {
    jsonText.value = data === null || data === undefined ? '' : JSON.stringify(data, null, 2)
  } else if (type === 'form') {
    formData.value = (data as Record<string, string>) ?? {}
  } else if (type === 'multipart') {
    const body = (data ?? {}) as Partial<MultipartBody>
    const rows: TextFieldRow[] = Object.entries(body.fields ?? {}).map(([field, value]) => ({
      field,
      value,
      contentType: '',
    }))
    for (const part of body.textParts ?? []) {
      rows.push({ field: part.field, value: part.value, contentType: part.contentType ?? '' })
    }
    multipartFields.value = rows
    multipartFiles.value = (body.files ?? []).map((file) => ({
      field: file.field,
      filename: file.filename,
      contentType: file.contentType,
      contentBase64: file.contentBase64 ?? '',
      size: Math.floor(((file.contentBase64?.length ?? 0) * 3) / 4),
    }))
  } else {
    rawText.value = typeof data === 'string' ? data : ''
  }

  header.value = conf.header ?? {}
  params.value = conf.params ?? {}
  authType.value = conf.authorization?.type ?? ''
  authToken.value = conf.authorization?.token ?? ''
  timeout.value = conf.concurrency?.timeout ?? 3000
  count.value = conf.concurrency?.count ?? 50
  totalRequests.value = conf.concurrency?.totalRequests ?? 1000
  rate.value = conf.concurrency?.rate ?? 0
}

async function submit() {
  errorMsg.value = ''
  let config: TestConfig
  try {
    config = buildConfig()
  } catch (err) {
    errorMsg.value = (err as Error).message
    return
  }

  submitting.value = true
  try {
    const res = await startTest(config)
    rememberConfig(config)
    await router.push(`/jobs/${res.jobId}`)
  } catch (err) {
    errorMsg.value = err instanceof ApiError ? err.message : (err as Error).message
  } finally {
    submitting.value = false
  }
}

/** 恢复上一次成功提交的配置（文件内容不会恢复） */
function restoreLast() {
  const raw = localStorage.getItem(LAST_CONFIG_KEY)
  if (!raw) {
    errorMsg.value = '还没有保存过的配置'
    return
  }
  try {
    const conf = JSON.parse(raw) as TestConfig
    applyConfig(conf)
    notice.value = '已恢复上次提交的参数（上传文件需重新选择）'
    errorMsg.value = ''
    sourceJob.value = ''
  } catch {
    errorMsg.value = '上次配置已损坏，无法恢复'
  }
}

onMounted(async () => {
  const from = route.query.from
  if (typeof from !== 'string' || !from) return

  try {
    const res = await getJobConfig(from)
    applyConfig(res.config)
    sourceJob.value = from
    notice.value = `已载入任务 ${from} 的完整参数，确认后可直接重新压测。`
    errorMsg.value = ''
  } catch (err) {
    errorMsg.value =
      err instanceof ApiError ? `加载任务 ${from} 的参数失败：${err.message}` : (err as Error).message
  }
})
</script>

<template>
  <div class="stack">
    <div v-if="notice" class="banner banner-info">
      <span>ℹ️</span>
      <span>{{ notice }}</span>
      <div class="spacer" />
      <span v-if="sourceJob" class="mono-sm">来源任务 {{ sourceJob }}</span>
    </div>

    <div v-if="runningCount > 0" class="banner banner-info">
      <span>📡</span>
      <span>
        当前已有 {{ runningCount }} 个任务在跑（后端最多同时运行 4 个），继续提交可能会被拒绝。
      </span>
    </div>

    <div v-if="errorMsg" class="banner banner-err">
      <span>⚠️</span>
      <span>{{ errorMsg }}</span>
    </div>

    <!-- 请求地址 -->
    <section class="card">
      <div class="card-head">
        <h2>请求目标</h2>
        <button class="btn btn-sm" type="button" @click="restoreLast">恢复上次配置</button>
      </div>
      <div class="card-body stack">
        <div class="row">
          <div class="field" style="flex: 1 1 340px">
            <label for="url">请求地址 <span style="color: var(--err)">*</span></label>
            <input
              id="url"
              v-model="url"
              class="input"
              placeholder="https://api.example.com/users"
              spellcheck="false"
            />
            <span class="desc">以 http:// 或 https:// 开头的完整地址</span>
          </div>
          <div class="field" style="flex: 0 0 130px">
            <label for="method">方法</label>
            <select id="method" v-model="method" class="select">
              <option v-for="m in methods" :key="m" :value="m">{{ m }}</option>
            </select>
          </div>
        </div>

        <div class="field">
          <label>URL 查询参数</label>
          <KeyValueEditor
            v-model="params"
            key-placeholder="参数名（如 source）"
            value-placeholder="参数值（如 apl）"
            empty-hint="未设置查询参数"
          />
        </div>
      </div>
    </section>

    <!-- 请求体 -->
    <section class="card">
      <div class="card-head">
        <h2>请求体</h2>
        <div class="tabs">
          <button
            v-for="t in bodyTypes"
            :key="t.value"
            class="tab"
            :class="{ active: bodyType === t.value }"
            type="button"
            @click="bodyType = t.value"
          >
            {{ t.label }}
          </button>
        </div>
      </div>
      <div class="card-body stack">
        <div class="body-scope">
          <span class="dim" style="font-size: 12px">{{ currentBodyHint }}</span>
          <span class="scope-note">
            当前只发送 <span class="chip">{{ bodyType }}</span> 请求体，其它页签里填的内容不会被发送
          </span>
        </div>

        <!-- json -->
        <div v-if="bodyType === 'json'" class="field">
          <span class="desc">
            按 JSON 解析后发送：<code>"abc"</code> 是字符串、<code>123</code> 是数字、
            <code>true</code> 是布尔
          </span>
          <textarea v-model="jsonText" class="textarea" spellcheck="false" />

          <div class="json-status" :class="jsonAnalysis.ok ? 'ok' : 'err'">
            {{ jsonAnalysis.message }}
          </div>

          <div v-if="jsonAnalysis.fields.length" class="type-list">
            <div v-for="row in jsonAnalysis.fields" :key="row.key" class="type-row">
              <span class="mono-sm type-key">{{ row.key }}</span>
              <span class="chip">{{ row.type }}</span>
              <span class="dim mono-sm type-preview">{{ row.preview }}</span>
            </div>
          </div>
        </div>

        <!-- form -->
        <div v-else-if="bodyType === 'form'" class="field">
          <span class="desc">
            以 application/x-www-form-urlencoded 发送；该格式没有类型概念，所有值都会变成字符串
          </span>
          <KeyValueEditor v-model="formData" empty-hint="未设置表单字段" />
        </div>

        <!-- multipart -->
        <div v-else-if="bodyType === 'multipart'" class="stack">
          <div class="field">
            <label>文本字段</label>
            <span class="desc">
              第三列可指定该部分的 Content-Type（例如把一段 JSON 以 application/json 发送），
              留空则按普通表单字段发送
            </span>
            <MultipartFieldsEditor v-model="multipartFields" />
          </div>

          <div class="field">
            <label>文件字段</label>
            <div class="row">
              <input
                v-model="pendingFileField"
                class="input"
                style="flex: 0 0 200px"
                placeholder="字段名（如 file）"
                spellcheck="false"
              />
              <button class="btn btn-sm" type="button" @click="pickFile">选择文件…</button>
              <span class="desc">单文件上限 10 MB，内容以 base64 提交给后端</span>
            </div>
            <input
              ref="fileInput"
              type="file"
              multiple
              style="display: none"
              @change="onFileChange"
            />

            <div v-if="multipartFiles.length" class="file-list">
              <div v-for="(file, index) in multipartFiles" :key="`${file.filename}-${index}`" class="file-row">
                <span class="chip">{{ file.field }}</span>
                <span class="file-name">{{ file.filename }}</span>
                <span class="dim mono-sm">{{ file.contentType }}</span>
                <span class="dim mono-sm">{{ formatBytes(file.size) }}</span>
                <button class="btn btn-ghost btn-sm" type="button" @click="removeFile(index)">
                  ✕
                </button>
              </div>
            </div>
            <div v-else class="desc">还没有添加文件</div>
          </div>
        </div>

        <!-- text / raw -->
        <div v-else class="field">
          <span class="desc">纯文本请求体，按 text/plain 发送</span>
          <textarea v-model="rawText" class="textarea" spellcheck="false" />
        </div>
      </div>
    </section>

    <!-- 请求头 + 认证 -->
    <div class="grid-2">
      <section class="card">
        <div class="card-head">
          <h2>请求头</h2>
        </div>
        <div class="card-body">
          <KeyValueEditor
            v-model="header"
            key-placeholder="Header 名"
            value-placeholder="Header 值"
            empty-hint="未设置请求头"
          />
        </div>
      </section>

      <section class="card">
        <div class="card-head">
          <h2>认证</h2>
        </div>
        <div class="card-body stack">
          <div class="field">
            <label for="authType">认证方式</label>
            <select id="authType" v-model="authType" class="select">
              <option value="">不认证</option>
              <option value="bearer">Bearer Token</option>
              <option value="basic">Basic（user:pass）</option>
              <option value="apikey">API Key（X-API-Key）</option>
            </select>
          </div>
          <div v-if="authType" class="field">
            <label for="authToken">凭据</label>
            <input
              id="authToken"
              v-model="authToken"
              class="input"
              :placeholder="authType === 'basic' ? 'user:pass' : 'token'"
              spellcheck="false"
            />
            <span class="desc">结果文件里会打码；配置存档（config_test）里保存原值，便于重新测试</span>
          </div>
        </div>
      </section>
    </div>

    <!-- 并发与落盘 -->
    <section class="card">
      <div class="card-head">
        <h2>并发与落盘</h2>
        <span class="hint">后端同时最多运行 4 个任务</span>
      </div>
      <div class="card-body stack">
        <div class="form-grid">
          <div class="field">
            <label for="timeout">单请求超时 (ms)</label>
            <input id="timeout" v-model.number="timeout" class="input" type="number" min="1" />
          </div>
          <div class="field">
            <label for="count">最大并发</label>
            <input id="count" v-model.number="count" class="input" type="number" min="1" />
          </div>
          <div class="field">
            <label for="total">总请求数</label>
            <input id="total" v-model.number="totalRequests" class="input" type="number" min="1" />
          </div>
          <div class="field">
            <label for="rate">每秒请求数 (rate)</label>
            <input id="rate" v-model.number="rate" class="input" type="number" min="0" />
            <span class="desc">后端暂未实现限速，当前不生效</span>
          </div>
        </div>

        <div class="field">
          <label>结果落盘</label>
          <div class="save-options">
            <label
              v-for="opt in saveDataOptions"
              :key="opt.value"
              class="save-option"
              :class="{ active: saveData === opt.value }"
            >
              <input v-model="saveData" type="radio" :value="opt.value" />
              <span>
                <strong>{{ opt.label }}</strong>
                <em>{{ opt.desc }}</em>
              </span>
            </label>
          </div>
        </div>
      </div>
    </section>

    <div class="submit-bar">
      <div class="dim" style="font-size: 12px">
        提交后任务在后台异步执行，可随时在「运行监控」查看或取消。
        <span v-if="totalRequests > 0">
          本次共 {{ formatNumber(Math.round(totalRequests)) }} 个请求。
        </span>
      </div>
      <div class="spacer" />
      <button class="btn" type="button" @click="restoreLast">恢复上次配置</button>
      <button class="btn btn-primary" type="button" :disabled="submitting" @click="submit">
        <span v-if="submitting" class="spin" />
        {{ submitting ? '提交中…' : '开始压测' }}
      </button>
    </div>
  </div>
</template>

<style scoped>
.save-options {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(230px, 1fr));
  gap: 9px;
}

.save-option {
  display: flex;
  gap: 9px;
  align-items: flex-start;
  padding: 10px 12px;
  border-radius: var(--radius-sm);
  border: 1px solid var(--border);
  background: rgba(255, 255, 255, 0.025);
  cursor: pointer;
  transition: border-color 0.15s, background 0.15s;
}

.save-option.active {
  border-color: rgba(56, 189, 248, 0.45);
  background: rgba(56, 189, 248, 0.1);
}

.save-option input {
  margin-top: 3px;
  accent-color: var(--accent);
}

.save-option strong {
  display: block;
  font-size: 12.5px;
  font-weight: 600;
}

.save-option em {
  display: block;
  font-style: normal;
  font-size: 11.5px;
  color: var(--dim);
  margin-top: 2px;
}

.submit-bar {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 13px 16px;
  border-radius: var(--radius);
  background: rgba(255, 255, 255, 0.035);
  border: 1px solid var(--border);
  position: sticky;
  bottom: 0;
  backdrop-filter: blur(10px);
}

.body-scope {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 10px;
}

.scope-note {
  font-size: 11.5px;
  color: var(--muted);
  padding: 3px 10px;
  border: 1px dashed var(--border-strong);
  border-radius: 999px;
}

.json-status {
  font-size: 12px;
  font-family: 'JetBrains Mono', Consolas, monospace;
}.json-status.ok {
  color: var(--ok);
}

.json-status.err {
  color: var(--err);
}

.type-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 9px 11px;
  border-radius: var(--radius-sm);
  border: 1px solid var(--border);
  background: rgba(0, 0, 0, 0.22);
  max-height: 190px;
  overflow: auto;
}

.type-row {
  display: grid;
  grid-template-columns: minmax(80px, 1fr) 76px minmax(0, 1.5fr);
  align-items: center;
  gap: 9px;
}

.type-key {
  color: var(--text);
  word-break: break-all;
}

.type-preview {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.file-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-top: 8px;
}

.file-row {
  display: grid;
  grid-template-columns: 90px minmax(0, 1fr) 140px 80px 30px;
  align-items: center;
  gap: 9px;
  padding: 6px 9px;
  border-radius: var(--radius-sm);
  border: 1px solid var(--border);
  background: rgba(255, 255, 255, 0.025);
}

.file-name {
  font-size: 12.5px;
  word-break: break-all;
}
</style>
