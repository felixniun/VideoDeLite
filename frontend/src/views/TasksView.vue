<script setup lang="ts">
import { computed, ref } from 'vue'
import { useTaskStore } from '../stores/tasks'
import { call } from '../services/wails'
import { t } from '../i18n'
import type { TaskView } from '../types'

const tasks = useTaskStore()
const revealErrors = ref<Record<string, string>>({})

const stateKey: Record<string, string> = {
  Waiting: 'tasks.waiting',
  Preparing: 'tasks.preparing',
  Encoding: 'tasks.encoding',
  Paused: 'tasks.paused',
  Validating: 'tasks.validating',
  Completed: 'tasks.completed',
  Canceled: 'tasks.canceled',
  Failed: 'tasks.failed'
}

const badgeClass: Record<string, string> = {
  Waiting: 'badge-gray',
  Preparing: 'badge-blue',
  Encoding: 'badge-blue',
  Paused: 'badge-orange',
  Validating: 'badge-blue',
  Completed: 'badge-green',
  Canceled: 'badge-gray',
  Failed: 'badge-red'
}

const activeIdx = computed(() => tasks.tasks.findIndex((x) => ['Encoding', 'Paused', 'Validating'].includes(x.state)))

function isTerminal(x: TaskView) {
  return ['Completed', 'Failed', 'Canceled'].includes(x.state)
}

function fmtTime(sec: number): string {
  if (sec <= 0 || !isFinite(sec)) return '—'
  const m = Math.floor(sec / 60)
  const s = Math.round(sec % 60)
  if (m >= 60) return `${Math.floor(m / 60)}h ${m % 60}m`
  return `${m}:${String(s).padStart(2, '0')}`
}

function fmtSize(bytes: number): string {
  if (bytes >= 1024 ** 3) return (bytes / 1024 ** 3).toFixed(2) + ' GB'
  if (bytes >= 1024 ** 2) return (bytes / 1024 ** 2).toFixed(1) + ' MB'
  return Math.max(1, Math.round(bytes / 1024)) + ' KB'
}

async function reveal(taskId: string, path: string) {
  try {
    await call('RevealPath', path)
    delete revealErrors.value[taskId]
  } catch (err) {
    const detail = err instanceof Error ? err.message : String(err)
    revealErrors.value[taskId] = `${t('tasks.revealFailed')}: ${detail}`
  }
}
</script>

<template>
  <div class="tasks">
    <h1 class="page-title">{{ t('tasks.title') }}</h1>

    <div v-if="tasks.tasks.length === 0" class="empty card">
      <div class="empty-icon">⚡</div>
      <div class="empty-title">{{ t('tasks.empty') }}</div>
      <div class="empty-hint">{{ t('tasks.emptyHint') }}</div>
    </div>

    <div
      v-for="(task, idx) in tasks.tasks"
      :key="task.id"
      class="task-card card"
      :class="{ current: idx === activeIdx }"
    >
      <div class="task-head">
        <div class="task-name" :title="task.inputPath">{{ task.inputName }}</div>
        <span class="badge" :class="badgeClass[task.state]">{{ t(stateKey[task.state]) }}</span>
      </div>

      <div class="task-meta">
        <span class="idx" v-if="idx === activeIdx">{{ t('tasks.of', { i: activeIdx + 1 }) }}</span>
        {{ task.codec.toUpperCase() }} · {{ task.container.toUpperCase() }} · {{ task.encoder }}
        <template v-if="task.quality"> · {{ task.quality }}</template>
        · {{ fmtSize(task.inputSize) }}
        <span v-if="task.fallbackUsed" class="badge badge-orange fb">{{ t('tasks.fallback') }}</span>
      </div>

      <div v-if="!isTerminal(task)" class="task-progress">
        <div class="progress-track">
          <div class="progress-fill" :style="{ width: (task.percent * 100).toFixed(1) + '%' }"></div>
        </div>
        <div class="progress-meta">
          <span>{{ (task.percent * 100).toFixed(1) }}%</span>
          <span v-if="task.speed > 0">{{ task.speed.toFixed(2) }}x</span>
          <span v-if="task.bitrateKbps > 0">{{ (task.bitrateKbps / 1000).toFixed(1) }} Mbps</span>
          <span v-if="task.etaSec > 0">{{ t('tasks.eta', { time: fmtTime(task.etaSec) }) }}</span>
        </div>
      </div>

      <div v-if="task.state === 'Failed'" class="task-error">
        {{ task.errCode }}: {{ task.errMessage }}
      </div>

      <div v-if="task.warnings.length" class="task-warnings">
        <div v-for="(w, i) in task.warnings" :key="i" class="warning-line">⚠ {{ w }}</div>
      </div>

      <div v-if="revealErrors[task.id]" class="task-error">{{ revealErrors[task.id] }}</div>

      <div class="task-actions">
        <template v-if="task.state === 'Encoding'">
          <button class="btn btn-secondary btn-sm" @click="tasks.pause(task.id)">{{ t('tasks.pause') }}</button>
          <button class="btn btn-danger btn-sm" @click="tasks.cancel(task.id)">{{ t('tasks.cancel') }}</button>
        </template>
        <template v-else-if="task.state === 'Paused'">
          <button class="btn btn-primary btn-sm" @click="tasks.resume(task.id)">{{ t('tasks.resume') }}</button>
          <button class="btn btn-danger btn-sm" @click="tasks.cancel(task.id)">{{ t('tasks.cancel') }}</button>
        </template>
        <template v-else-if="task.state === 'Waiting' || task.state === 'Preparing' || task.state === 'Validating'">
          <button class="btn btn-danger btn-sm" @click="tasks.cancel(task.id)">{{ t('tasks.cancel') }}</button>
        </template>
        <template v-else>
          <button v-if="task.outputPath" class="btn btn-secondary btn-sm" @click="reveal(task.id, task.outputPath)">{{ t('tasks.reveal') }}</button>
          <button v-if="task.state !== 'Completed'" class="btn btn-ghost btn-sm" @click="tasks.retry(task.id)">{{ t('tasks.retry') }}</button>
          <button class="btn btn-ghost btn-sm" @click="tasks.remove(task.id)">{{ t('tasks.remove') }}</button>
        </template>
      </div>
    </div>
  </div>
</template>

<style scoped>
.tasks { max-width: 860px; margin: 0 auto; }

.page-title {
  font-size: 26px;
  font-weight: 700;
  letter-spacing: -0.3px;
  margin-bottom: 18px;
}

.empty { text-align: center; padding: 60px 24px; }
.empty-icon { font-size: 36px; margin-bottom: 10px; }
.empty-title { font-size: 16px; font-weight: 600; }
.empty-hint { font-size: 13px; color: var(--text-secondary); margin-top: 4px; }

.task-card {
  padding: 16px 20px;
  margin-bottom: 14px;
  transition: box-shadow 0.2s;
}

.task-card.current {
  box-shadow: 0 0 0 2px var(--accent-soft), var(--shadow-card);
}

.task-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.task-name {
  font-size: 14.5px;
  font-weight: 620;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.task-meta {
  font-size: 12.5px;
  color: var(--text-secondary);
  margin-top: 4px;
}

.idx { color: var(--accent); font-weight: 650; }

.fb { margin-left: 6px; }

.task-progress { margin-top: 12px; }

.progress-meta {
  display: flex;
  gap: 14px;
  font-size: 12px;
  color: var(--text-secondary);
  margin-top: 6px;
}

.task-error {
  margin-top: 10px;
  padding: 10px 14px;
  border-radius: var(--radius-md);
  background: rgba(255, 69, 58, 0.1);
  color: var(--red);
  font-size: 12.5px;
  word-break: break-all;
}

.task-warnings { margin-top: 8px; }

.warning-line {
  font-size: 12px;
  color: var(--orange);
  margin-top: 3px;
}

.task-actions {
  display: flex;
  gap: 8px;
  margin-top: 12px;
  justify-content: flex-end;
}
</style>
