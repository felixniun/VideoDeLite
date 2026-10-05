<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { call } from '../services/wails'
import { t } from '../i18n'
import type { HistoryEntry } from '../types'

const entries = ref<HistoryEntry[]>([])
const loaded = ref(false)

async function load() {
  try {
    entries.value = (await call<HistoryEntry[]>('GetHistory')) ?? []
  } catch { /* browser dev */ }
  loaded.value = true
}

onMounted(load)

async function clearHistory() {
  if (!confirm(t('history.clearConfirm'))) return
  try {
    await call('ClearHistory')
    await load()
  } catch { /* noop */ }
}

async function reveal(path: string) {
  try { await call('RevealPath', path) } catch { /* noop */ }
}

function fmtSize(bytes: number): string {
  if (!bytes) return '—'
  if (bytes >= 1024 ** 3) return (bytes / 1024 ** 3).toFixed(2) + ' GB'
  if (bytes >= 1024 ** 2) return (bytes / 1024 ** 2).toFixed(1) + ' MB'
  return Math.max(1, Math.round(bytes / 1024)) + ' KB'
}

function fmtTime(sec: number): string {
  if (sec <= 0) return '—'
  const m = Math.floor(sec / 60)
  const s = Math.round(sec % 60)
  return `${m}:${String(s).padStart(2, '0')}`
}

const statusBadge: Record<string, string> = {
  Completed: 'badge-green',
  Failed: 'badge-red',
  Canceled: 'badge-gray'
}
</script>

<template>
  <div class="history">
    <div class="head-row">
      <h1 class="page-title">{{ t('history.title') }}</h1>
      <button v-if="entries.length" class="btn btn-secondary btn-sm" @click="clearHistory">
        {{ t('history.clear') }}
      </button>
    </div>

    <div v-if="loaded && entries.length === 0" class="empty card">
      <div class="empty-icon">🕘</div>
      <div class="empty-title">{{ t('history.empty') }}</div>
    </div>

    <div class="list card" v-if="entries.length">
      <div v-for="e in entries" :key="e.id" class="row">
        <div class="main">
          <div class="name" :title="e.inputPath">{{ e.inputName }}</div>
          <div class="meta">
            {{ e.createdAt }} ·
            {{ (e.videoCodec || '').toUpperCase() || '—' }} · {{ e.encoder || '—' }}
            <template v-if="e.quality"> · {{ e.quality }}</template>
            <span v-if="e.warnings" class="warn-dot" :title="e.warnings">⚠</span>
          </div>
        </div>
        <div class="stats">
          <span class="stat">{{ fmtSize(e.inputSize) }} → {{ fmtSize(e.outputSize) }}</span>
          <span v-if="e.ratio > 0" class="stat ratio">-{{ ((1 - e.ratio) * 100).toFixed(0) }}%</span>
          <span class="stat">{{ fmtTime(e.encodeTimeSec) }}</span>
          <span class="badge" :class="statusBadge[e.status] || 'badge-gray'">{{ e.status }}</span>
        </div>
        <button v-if="e.outputPath" class="btn btn-ghost btn-sm" @click="reveal(e.outputPath)">↗</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.history { max-width: 920px; margin: 0 auto; }

.head-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 18px;
}

.page-title {
  font-size: 26px;
  font-weight: 700;
  letter-spacing: -0.3px;
}

.empty { text-align: center; padding: 60px 24px; }
.empty-icon { font-size: 36px; margin-bottom: 8px; }
.empty-title { font-size: 15px; color: var(--text-secondary); }

.list { padding: 4px 20px; }

.row {
  display: flex;
  align-items: center;
  gap: 16px;
  padding: 12px 0;
  border-bottom: 1px solid var(--separator);
}

.row:last-child { border-bottom: none; }

.main { flex: 1; min-width: 0; }

.name {
  font-size: 13.5px;
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.meta {
  font-size: 12px;
  color: var(--text-secondary);
  margin-top: 2px;
}

.warn-dot { color: var(--orange); margin-left: 6px; }

.stats {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-shrink: 0;
}

.stat { font-size: 12.5px; color: var(--text-secondary); }

.ratio {
  color: var(--green);
  font-weight: 700;
}
</style>
