<script setup lang="ts">
import { computed } from 'vue'
import type { RecordLine } from '../api/types'
import {
  codeClass,
  errKindText,
  failureKindText,
  formatBytes,
  formatMs,
  formatTime,
} from '../utils/format'

const props = defineProps<{ line: RecordLine | null }>()
const emit = defineEmits<{ close: [] }>()

const visible = computed(() => props.line !== null)

const status = computed(() => props.line?.status ?? props.line?.s ?? 0)

/** 简写记录（saveData=2 的成功行）没有 kind 字段，用状态码推断 */
const kindLabel = computed(() => {
  if (props.line?.kind) return failureKindText(props.line.kind)
  const code = status.value
  if (!code) return '—'
  return code >= 200 && code < 300 ? '成功' : '非 2xx'
})

/** 把 JSON 字符串美化，失败时原样返回 */
function pretty(text: string | undefined): string {
  if (!text) return ''
  try {
    return JSON.stringify(JSON.parse(text), null, 2)
  } catch {
    return text
  }
}

function headerText(headers: Record<string, string> | undefined): string {
  if (!headers) return ''
  return Object.entries(headers)
    .map(([key, value]) => `${key}: ${value}`)
    .join('\n')
}
</script>

<template>
  <Teleport to="body">
    <div v-if="visible" class="drawer-mask" @click.self="emit('close')">
      <aside class="drawer">
        <header class="drawer-head">
          <div class="drawer-title">
            <span class="mono">#{{ line?.i }}</span>
            <span
              class="badge"
              :class="kindLabel === '成功' ? 'badge-ok' : 'badge-err'"
            >{{ kindLabel }}</span>
            <span :class="codeClass(status)">{{ status || '—' }}</span>
            <span class="dim mono-sm">{{ formatMs(line?.ms) }}</span>
          </div>
          <button class="btn btn-ghost btn-sm" @click="emit('close')">关闭 ✕</button>
        </header>

        <div class="drawer-body">
          <dl class="kv">
            <template v-if="line?.method || line?.url">
              <dt>请求</dt>
              <dd>{{ line?.method || '—' }} {{ line?.url || '—' }}</dd>
            </template>
            <dt>发生时间</dt>
            <dd>{{ formatTime(line?.at) }}</dd>
            <template v-if="line?.err">
              <dt>错误信息</dt>
              <dd style="color: #fca5a5">{{ line.err }}</dd>
            </template>
            <template v-if="line?.errKind">
              <dt>错误归类</dt>
              <dd>{{ errKindText(line.errKind) }}</dd>
            </template>
            <dt>响应字节</dt>
            <dd>{{ formatBytes(line?.respBytes ?? line?.size) }}</dd>
          </dl>

          <section v-if="line?.respBody" class="drawer-section">
            <h4>
              响应体
              <span v-if="line.respBodyTruncated" class="chip">已截断</span>
            </h4>
            <pre class="block">{{ pretty(line.respBody) }}</pre>
          </section>

          <section v-if="line?.respHeader" class="drawer-section">
            <h4>响应头</h4>
            <pre class="block">{{ headerText(line.respHeader) }}</pre>
          </section>

          <section v-if="line?.reqBody" class="drawer-section">
            <h4>
              请求体
              <span v-if="line.reqBodyTruncated" class="chip">已截断</span>
            </h4>
            <pre class="block">{{ pretty(line.reqBody) }}</pre>
          </section>

          <section v-if="line?.reqHeader" class="drawer-section">
            <h4>请求头</h4>
            <pre class="block">{{ headerText(line.reqHeader) }}</pre>
          </section>

          <div v-if="!line?.respBody && !line?.reqBody" class="dim" style="font-size: 12px">
            该记录不含请求/响应体（saveData=2 时成功请求只记简写信息）。
          </div>
        </div>
      </aside>
    </div>
  </Teleport>
</template>

<style scoped>
.drawer-mask {
  position: fixed;
  inset: 0;
  background: rgba(4, 8, 16, 0.62);
  backdrop-filter: blur(2px);
  z-index: 50;
  display: flex;
  justify-content: flex-end;
}

.drawer {
  width: min(620px, 92vw);
  height: 100%;
  background: #0c1524;
  border-left: 1px solid var(--border);
  display: flex;
  flex-direction: column;
  box-shadow: -20px 0 50px rgba(0, 0, 0, 0.45);
  animation: slide-in 0.18s ease-out;
}

@keyframes slide-in {
  from {
    transform: translateX(24px);
    opacity: 0.4;
  }
  to {
    transform: translateX(0);
    opacity: 1;
  }
}

.drawer-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 13px 16px;
  border-bottom: 1px solid var(--border);
}

.drawer-title {
  display: flex;
  align-items: center;
  gap: 9px;
  flex-wrap: wrap;
  font-size: 13px;
}

.drawer-body {
  padding: 16px;
  overflow: auto;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.drawer-section h4 {
  margin: 0 0 7px;
  font-size: 12px;
  color: var(--muted);
  font-weight: 600;
  display: flex;
  align-items: center;
  gap: 7px;
}
</style>
