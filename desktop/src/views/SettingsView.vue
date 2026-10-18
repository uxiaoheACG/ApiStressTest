<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { getBaseUrl, health, resetBaseUrl, setBaseUrl } from '../api/client'

const baseUrl = ref(getBaseUrl())
const saved = ref('')
const testing = ref(false)
const testResult = ref<{ ok: boolean; message: string } | null>(null)

const backendBusy = ref(false)
const backendStatus = ref<{ running: boolean; pid?: number; exists: boolean } | null>(null)
const backendMessage = ref('')
const runtime = ref<{
  electron: string
  platform: string
  backendUrl: string
  dataDir: string
  dataDirKnown: boolean
} | null>(null)

const dataDirMessage = ref('')

const userAgent = navigator.userAgent

const hasBridge = typeof window !== 'undefined' && Boolean(window.ast)

async function save() {
  setBaseUrl(baseUrl.value)
  baseUrl.value = getBaseUrl()
  saved.value = '已保存'
  await test()
  window.setTimeout(() => (saved.value = ''), 2000)
}

async function reset() {
  resetBaseUrl()
  baseUrl.value = getBaseUrl()
  saved.value = '已恢复默认'
  await test()
  window.setTimeout(() => (saved.value = ''), 2000)
}

async function test() {
  testing.value = true
  testResult.value = null
  try {
    const res = await health()
    testResult.value = {
      ok: res.status === 'OK',
      message: res.status === 'OK' ? '连接正常（GET / 返回 OK）' : `异常响应: ${res.status}`,
    }
  } catch (err) {
    testResult.value = { ok: false, message: (err as Error).message }
  } finally {
    testing.value = false
  }
}

async function refreshBackend() {
  if (!window.ast) return
  try {
    backendStatus.value = await window.ast.backend.status()
  } catch {
    backendStatus.value = null
  }
}

async function startBackend() {
  if (!window.ast) return
  backendBusy.value = true
  backendMessage.value = ''
  try {
    const res = await window.ast.backend.start()
    backendMessage.value = res.message
    await refreshBackend()
    await test()
  } finally {
    backendBusy.value = false
  }
}

async function stopBackend() {
  if (!window.ast) return
  backendBusy.value = true
  backendMessage.value = ''
  try {
    const res = await window.ast.backend.stop()
    backendMessage.value = res.message
    await refreshBackend()
  } finally {
    backendBusy.value = false
  }
}

async function openDataDir() {
  if (!window.ast) return
  const res = await window.ast.openDataDir()
  dataDirMessage.value = res.message ?? (res.ok ? '已打开结果目录' : '打开失败')
  window.setTimeout(() => (dataDirMessage.value = ''), 4000)
}

onMounted(async () => {
  await test()
  if (window.ast) {
    runtime.value = await window.ast.runtime()
    await refreshBackend()
  }
})
</script>

<template>
  <div class="stack">
    <section class="card">
      <div class="card-head">
        <h2>后端连接</h2>
        <span v-if="saved" class="badge badge-ok">{{ saved }}</span>
      </div>
      <div class="card-body stack">
        <div class="row">
          <div class="field" style="flex: 1 1 320px">
            <label for="baseUrl">后端地址</label>
            <input id="baseUrl" v-model="baseUrl" class="input" spellcheck="false" />
            <span class="desc">默认 http://127.0.0.1:3010，与 serverConfig.yaml 的 addr 对应</span>
          </div>
          <button class="btn btn-primary" :disabled="testing" @click="save">
            {{ testing ? '连接中…' : '保存并测试' }}
          </button>
          <button class="btn" @click="reset">恢复默认</button>
        </div>

        <div
          v-if="testResult"
          class="banner"
          :class="testResult.ok ? 'banner-info' : 'banner-err'"
        >
          <span>{{ testResult.ok ? '✅' : '⚠️' }}</span>
          <span>{{ testResult.message }}</span>
        </div>
      </div>
    </section>

    <section class="card">
      <div class="card-head">
        <h2>内置后端进程</h2>
        <span class="hint">
          {{ hasBridge ? '由桌面应用管理 ast.exe' : '当前是浏览器环境，无法管理进程' }}
        </span>
      </div>
      <div class="card-body stack">
        <div class="grid-3">
          <div class="stat">
            <div class="stat-label">运行状态</div>
            <div class="stat-value" :class="backendStatus?.running ? 'ok' : 'warn'">
              {{ backendStatus ? (backendStatus.running ? '运行中' : '未运行') : '未知' }}
            </div>
            <div class="stat-sub">
              <span v-if="backendStatus?.pid">PID {{ backendStatus.pid }}</span>
              <span v-else-if="backendStatus && !backendStatus.exists">未找到 ast.exe</span>
            </div>
          </div>
          <div class="stat">
            <div class="stat-label">后端地址</div>
            <div class="stat-value mono-sm" style="font-size: 14px">
              {{ runtime?.backendUrl ?? baseUrl }}
            </div>
          </div>
          <div class="stat">
            <div class="stat-label">运行环境</div>
            <div class="stat-value" style="font-size: 14px">
              {{ runtime ? `Electron ${runtime.electron}` : '浏览器' }}
            </div>
            <div class="stat-sub">{{ runtime?.platform ?? userAgent }}</div>
          </div>
        </div>

        <div class="row" style="align-items: stretch">
          <div class="field" style="flex: 1 1 320px">
            <label>结果数据目录</label>
            <div class="path-box">{{ runtime?.dataDir ?? '启动后端后确定' }}</div>
            <span class="desc">
              每个任务一个子目录，内含 summary.json / success.jsonl / failures.jsonl；
              默认在应用安装目录同级的 ast-data 下（卸载/重装不受影响），
              该位置不可写时自动改用用户数据目录
            </span>
          </div>
          <button class="btn" :disabled="!hasBridge" @click="openDataDir">打开目录</button>
        </div>

        <div v-if="dataDirMessage" class="banner banner-info">
          <span>📁</span><span>{{ dataDirMessage }}</span>
        </div>

        <div v-if="backendMessage" class="banner banner-info">
          <span>ℹ️</span><span>{{ backendMessage }}</span>
        </div>

        <div class="row">
          <button class="btn" :disabled="!hasBridge || backendBusy" @click="startBackend">
            启动后端
          </button>
          <button class="btn btn-danger" :disabled="!hasBridge || backendBusy" @click="stopBackend">
            停止后端
          </button>
          <button class="btn btn-ghost" :disabled="!hasBridge" @click="refreshBackend">
            刷新状态
          </button>
        </div>

        <div class="desc">
          桌面应用启动时会自动尝试拉起随包的 ast.exe；若本机已有后端在 3010 运行，则直接复用，
          不会重复启动。停止后需要手动再次启动。
        </div>
      </div>
    </section>

    <section class="card">
      <div class="card-head">
        <h2>使用说明</h2>
      </div>
      <div class="card-body">
        <dl class="kv">
          <dt>结果目录</dt>
          <dd>上方「结果数据目录」显示的真实路径：默认在安装目录同级的 ast-data\Data\&lt;时间&gt;_&lt;jobID&gt;，
            卸载/覆盖安装不会清理；该位置不可写时改用 %APPDATA%\ast-stress-desktop\Data</dd>
          <dt>参数存档</dt>
          <dd>每次压测的完整参数（含 token 与上传文件）写入 config_test/&lt;jobID&gt;.json，
            历史详情页可查看并一键重新测试</dd>
          <dt>端口</dt>
          <dd>默认监听 3010，可在 serverConfig.yaml 的 addr 中修改（旧版默认 8080）</dd>
          <dt>并发上限</dt>
          <dd>后端同时最多运行 4 个任务，超出返回 429</dd>
          <dt>内存任务</dt>
          <dd>GET /jobs 只保留最近 50 条，进程重启即清空；历史结果请查 GET /history</dd>
          <dt>rate 参数</dt>
          <dd>后端尚未实现按秒限速，填写后不生效（仅透传）</dd>
          <dt>凭证安全</dt>
          <dd>Authorization / Cookie / X-API-Key 在结果文件中只保留前 4 位，其余打码</dd>
        </dl>
      </div>
    </section>
  </div>
</template>

<style scoped>
.path-box {
  padding: 8px 11px;
  border-radius: var(--radius-sm);
  border: 1px solid var(--border);
  background: rgba(0, 0, 0, 0.28);
  font-family: 'JetBrains Mono', Consolas, monospace;
  font-size: 12px;
  word-break: break-all;
}
</style>
