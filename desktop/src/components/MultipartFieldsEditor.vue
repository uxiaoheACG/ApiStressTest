<script setup lang="ts">
import { computed } from 'vue'
import type { TextFieldRow } from '../api/types'

const props = defineProps<{ modelValue: TextFieldRow[] }>()
const emit = defineEmits<{ 'update:modelValue': [TextFieldRow[]] }>()

const rows = computed(() => props.modelValue)

function add() {
  emit('update:modelValue', [...props.modelValue, { field: '', value: '', contentType: '' }])
}

function remove(index: number) {
  emit(
    'update:modelValue',
    props.modelValue.filter((_, i) => i !== index),
  )
}

function setValue(index: number, key: keyof TextFieldRow, value: string) {
  const next = props.modelValue.map((row, i) => (i === index ? { ...row, [key]: value } : row))
  emit('update:modelValue', next)
}

const COMMON_TYPES = [
  'application/json',
  'text/plain',
  'text/html',
  'application/xml',
  'application/x-ndjson',
]
</script>

<template>
  <div class="mf-editor">
    <div v-if="rows.length === 0" class="mf-empty">未设置文本字段</div>

    <div v-for="(row, index) in rows" :key="index" class="mf-row">
      <input
        class="input"
        :value="row.field"
        placeholder="字段名"
        spellcheck="false"
        @input="setValue(index, 'field', ($event.target as HTMLInputElement).value)"
      />
      <input
        class="input"
        :value="row.value"
        placeholder="字段值（可直接粘 JSON）"
        spellcheck="false"
        @input="setValue(index, 'value', ($event.target as HTMLInputElement).value)"
      />
      <input
        class="input mf-type"
        :value="row.contentType"
        list="mf-content-types"
        placeholder="Content-Type（可选）"
        spellcheck="false"
        @input="setValue(index, 'contentType', ($event.target as HTMLInputElement).value)"
      />
      <button class="btn btn-ghost btn-sm" type="button" title="删除该字段" @click="remove(index)">
        ✕
      </button>
    </div>

    <datalist id="mf-content-types">
      <option v-for="type in COMMON_TYPES" :key="type" :value="type" />
    </datalist>

    <button class="btn btn-sm add-btn" type="button" @click="add">＋ 添加字段</button>
  </div>
</template>

<style scoped>
.mf-editor {
  display: flex;
  flex-direction: column;
  gap: 7px;
}

.mf-row {
  display: grid;
  grid-template-columns: minmax(0, 0.9fr) minmax(0, 1.4fr) minmax(0, 1.1fr) 30px;
  gap: 7px;
  align-items: center;
}

.mf-row :deep(.input) {
  padding: 6px 9px;
  font-size: 12.5px;
  font-family: 'JetBrains Mono', Consolas, monospace;
}

.mf-type {
  font-size: 12px;
}

.mf-empty {
  font-size: 12px;
  color: var(--dim);
  padding: 2px 0 4px;
}

.add-btn {
  align-self: flex-start;
  margin-top: 2px;
}
</style>
