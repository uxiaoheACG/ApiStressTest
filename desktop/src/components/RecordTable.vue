<script setup lang="ts">
import type { RecordKind, RecordLine } from '../api/types'
import { codeClass, errKindText, failureKindText, formatBytes, formatClock, formatMs } from '../utils/format'

defineProps<{
  lines: RecordLine[]
  kind: RecordKind
  loading?: boolean
}>()

const emit = defineEmits<{ select: [RecordLine] }>()

/** 简写行只有 s（状态码），详细行有 status */
function statusOf(line: RecordLine): number {
  return line.status ?? line.s ?? 0
}

function kindOf(line: RecordLine): string {
  if (line.kind) return line.kind
  const status = statusOf(line)
  return status >= 200 && status < 300 ? 'success' : 'unsuccess'
}

function sizeOf(line: RecordLine): number {
  return line.size ?? line.respBytes ?? 0
}
</script>

<template>
  <div class="table-wrap">
    <table class="table">
      <thead>
        <tr>
          <th style="width: 62px">序号</th>
          <th style="width: 96px">类型</th>
          <th style="width: 76px">状态码</th>
          <th style="width: 84px" class="num">耗时</th>
          <th v-if="kind === 'failures'" style="width: 96px">错误归类</th>
          <th style="width: 88px" class="num">响应大小</th>
          <th>时间</th>
        </tr>
      </thead>
      <tbody>
        <tr v-if="loading">
          <td colspan="7" style="text-align: center; padding: 22px">
            <span class="spin" /> <span class="dim">加载中…</span>
          </td>
        </tr>
        <tr v-else-if="lines.length === 0">
          <td colspan="7" class="empty">这一类还没有记录</td>
        </tr>
        <template v-else>
          <tr
            v-for="line in lines"
            :key="`${line.i}-${line.at}`"
            @click="emit('select', line)"
          >
            <td class="mono-sm">#{{ line.i }}</td>
            <td>
              <span
                class="badge"
                :class="kindOf(line) === 'success' ? 'badge-ok' : 'badge-err'"
              >{{ failureKindText(kindOf(line)) }}</span>
            </td>
            <td>
              <span :class="codeClass(statusOf(line))">{{ statusOf(line) || '—' }}</span>
            </td>
            <td class="num mono-sm">{{ formatMs(line.ms) }}</td>
            <td v-if="kind === 'failures'" class="mono-sm dim">
              {{ line.errKind ? errKindText(line.errKind) : '—' }}
            </td>
            <td class="num mono-sm">{{ formatBytes(sizeOf(line)) }}</td>
            <td class="mono-sm dim">{{ formatClock(line.at) }}</td>
          </tr>
        </template>
      </tbody>
    </table>
  </div>
</template>
