<script setup lang="ts">
import { ref, watch } from 'vue'

interface Row {
  key: string
  value: string
}

const props = withDefaults(
  defineProps<{
    modelValue: Record<string, string>
    keyPlaceholder?: string
    valuePlaceholder?: string
    emptyHint?: string
  }>(),
  {
    keyPlaceholder: '名称',
    valuePlaceholder: '值',
    emptyHint: '暂无条目',
  },
)

const emit = defineEmits<{ 'update:modelValue': [Record<string, string>] }>()

const rows = ref<Row[]>(toRows(props.modelValue))

function toRows(map: Record<string, string>): Row[] {
  return Object.entries(map ?? {}).map(([key, value]) => ({ key, value }))
}

function toMap(list: Row[]): Record<string, string> {
  const map: Record<string, string> = {}
  for (const row of list) {
    const key = row.key.trim()
    if (key) map[key] = row.value
  }
  return map
}

// 父组件从外部替换整份数据（例如「恢复上次配置」）时同步到本地行
watch(
  () => props.modelValue,
  (next) => {
    if (JSON.stringify(toMap(rows.value)) !== JSON.stringify(next ?? {})) {
      rows.value = toRows(next)
    }
  },
  { deep: true },
)

function addRow() {
  rows.value.push({ key: '', value: '' })
}

function removeRow(index: number) {
  rows.value.splice(index, 1)
  update()
}

function update() {
  emit('update:modelValue', toMap(rows.value))
}
</script>

<template>
  <div class="kv-editor">
    <div v-if="rows.length === 0" class="kv-empty">{{ emptyHint }}</div>

    <div v-for="(row, index) in rows" :key="index" class="kv-row">
      <input
        v-model="row.key"
        class="input"
        :placeholder="keyPlaceholder"
        spellcheck="false"
        @input="update"
      />
      <input
        v-model="row.value"
        class="input"
        :placeholder="valuePlaceholder"
        spellcheck="false"
        @input="update"
      />
      <button class="btn btn-ghost btn-sm" type="button" title="删除该行" @click="removeRow(index)">
        ✕
      </button>
    </div>

    <button class="btn btn-sm add-btn" type="button" @click="addRow">＋ 添加一项</button>
  </div>
</template>

<style scoped>
.kv-editor {
  display: flex;
  flex-direction: column;
  gap: 7px;
}

.kv-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1.4fr) 30px;
  gap: 7px;
  align-items: center;
}

.kv-row :deep(.input) {
  padding: 6px 9px;
  font-size: 12.5px;
  font-family: 'JetBrains Mono', Consolas, monospace;
}

.kv-empty {
  font-size: 12px;
  color: var(--dim);
  padding: 2px 0 4px;
}

.add-btn {
  align-self: flex-start;
  margin-top: 2px;
}
</style>
