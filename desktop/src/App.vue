<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { RouterLink, RouterView, useRoute } from 'vue-router'
import { getBaseUrl, health } from './api/client'

const route = useRoute()

const online = ref<boolean | null>(null)
const lastError = ref('')
const baseUrl = ref(getBaseUrl())
const checking = ref(false)
/** 手动检测后的短暂结果提示：ok / err，2.5 秒后自动清空 */
const flash = ref<'' | 'ok' | 'err'>('')
let timer: number | undefined
let flashTimer: number | undefined
/** 当前轮询间隔：离线时缩短，后端刚启动也能很快连上 */
let intervalMs = 6000

const navItems = [
  { to: '/dashboard', icon: '📊', label: '运行监控' },
  { to: '/new', icon: '⚡', label: '新建压测' },
  { to: '/history', icon: '🗂️', label: '历史记录' },
  { to: '/settings', icon: '⚙️', label: '设置' },
]

const title = computed(() => (route.meta.title as string | undefined) ?? 'AST 压测台')
const subtitle = computed(() => {
  if (online.value === null) return '正在检测后端…'
  if (online.value) return '后端已连接 · 数据实时刷新'
  return '后端未连接 · 请在设置页确认地址或启动后端'
})

async function check(manual = false) {
  baseUrl.value = getBaseUrl()
  checking.value = true
  try {
    const res = await health()
    online.value = res.status === 'OK'
    lastError.value = ''
  } catch (err) {
    online.value = false
    lastError.value = (err as Error).message
  } finally {
    checking.value = false
    // 只有手动点击才给“已连接 / 连接失败”的短暂反馈，自动轮询不打扰
    if (manual) {
      flash.value = online.value ? 'ok' : 'err'
      if (flashTimer) window.clearTimeout(flashTimer)
      flashTimer = window.setTimeout(() => {
        flash.value = ''
      }, 2500)
    }
    // 离线时 2.5 秒重试一次（后端可能正在启动），在线时回到 6 秒
    startTimer(online.value ? 6000 : 2500)
  }
}

/** 重设轮询间隔，间隔没变就不重建定时器 */
function startTimer(ms: number) {
  if (ms === intervalMs && timer !== undefined) return
  intervalMs = ms
  if (timer) window.clearInterval(timer)
  timer = window.setInterval(check, ms)
}

/** 顶部「重新检测」按钮：手动检测一次 */
function checkNow() {
  void check(true)
}

onMounted(() => {
  void check()
  startTimer(6000)
})

onUnmounted(() => {
  if (timer) window.clearInterval(timer)
  if (flashTimer) window.clearTimeout(flashTimer)
})
</script>

<template>
  <div class="app-shell">
    <aside class="sidebar">
      <div class="brand">
        <div class="brand-mark">AST</div>
        <div class="brand-text">
          <strong>压测台</strong>
          <span>API STRESS TEST</span>
        </div>
      </div>

      <nav class="nav">
        <RouterLink
          v-for="item in navItems"
          :key="item.to"
          :to="item.to"
          class="nav-item"
          active-class="active"
        >
          <span class="nav-icon">{{ item.icon }}</span>
          <span>{{ item.label }}</span>
        </RouterLink>
      </nav>

      <div class="sidebar-foot">
        <span>{{ baseUrl }}</span>
        <span>
          <span
            :class="online ? 'dot dot-ok' : online === false ? 'dot dot-err' : 'dot'"
          />
          {{ online ? '在线' : online === false ? '离线' : '检测中' }}
        </span>
      </div>
    </aside>

    <div class="main">
      <header class="topbar">
        <div>
          <h1>{{ title }}</h1>
          <div class="topbar-sub">{{ subtitle }}</div>
        </div>
        <div class="topbar-actions">
          <RouterLink to="/new" class="btn btn-primary btn-sm">＋ 新建压测</RouterLink>
          <button
            class="btn btn-sm detect-btn"
            :class="{ 'btn-flash-ok': flash === 'ok', 'btn-flash-err': flash === 'err' }"
            :disabled="checking"
            :title="lastError || '立即重新检测后端连接'"
            @click="checkNow"
          >
            <span v-if="checking" class="spin" />
            {{
              checking
                ? '检测中…'
                : flash === 'ok'
                  ? '✓ 已连接'
                  : flash === 'err'
                    ? '✕ 连接失败'
                    : '重新检测'
            }}
          </button>
        </div>
      </header>

      <main class="content">
        <div v-if="online === false" class="banner banner-err" style="margin-bottom: 16px">
          <span>⚠️</span>
          <div>
            连不上后端 <span class="mono-sm">{{ baseUrl }}</span>
            <span v-if="lastError" class="dim"> · {{ lastError }}</span>
          </div>
          <div class="spacer" />
          <RouterLink to="/settings" class="link">去设置</RouterLink>
        </div>

        <RouterView />
      </main>
    </div>
  </div>
</template>

<style scoped>
.dot {
  display: inline-block;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  margin-right: 5px;
  background: var(--dim);
  vertical-align: middle;
}

.detect-btn {
  min-width: 104px;
}

.btn-flash-ok {
  background: var(--ok-soft);
  border-color: rgba(52, 211, 153, 0.45);
  color: #6ee7b7;
}

.btn-flash-err {
  background: var(--err-soft);
  border-color: rgba(248, 113, 113, 0.45);
  color: #fca5a5;
}

.dot-ok {
  background: var(--ok);
  box-shadow: 0 0 6px rgba(52, 211, 153, 0.7);
}

.dot-err {
  background: var(--err);
  box-shadow: 0 0 6px rgba(248, 113, 113, 0.6);
}
</style>
