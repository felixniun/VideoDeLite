import { defineStore } from 'pinia'
import { ref } from 'vue'
import { call, onEvent } from '../services/wails'
import type { TaskView } from '../types'

export const useTaskStore = defineStore('tasks', () => {
  const tasks = ref<TaskView[]>([])
  let listening = false

  function upsert(t: TaskView) {
    const i = tasks.value.findIndex((x) => x.id === t.id)
    if (i >= 0) tasks.value[i] = t
    else tasks.value.push(t)
  }

  async function refresh() {
    try {
      tasks.value = await call<TaskView[]>('GetTasks')
    } catch { /* browser dev mode */ }
  }

  function listen() {
    if (listening) return
    listening = true
    onEvent('task:update', (t: TaskView) => upsert(t))
    onEvent('task:progress', (t: TaskView) => upsert(t))
    onEvent('task:removed', (d: { id: string }) => {
      tasks.value = tasks.value.filter((x) => x.id !== d.id)
    })
  }

  async function enqueue(files: string[], opts: {
    codec: string; quality: string; container: string; outputDir: string; encoder: string
  }) {
    return call<string[]>('CreateSimpleTasks', files, opts.codec, opts.quality, opts.container, opts.outputDir, opts.encoder)
  }

  async function pause(id: string) { await call('PauseTask', id); refresh() }
  async function resume(id: string) { await call('ResumeTask', id); refresh() }
  async function cancel(id: string) { await call('CancelTask', id) }
  async function remove(id: string) { await call('RemoveTask', id) }
  async function retry(id: string) { await call('RetryTask', id) }

  return { tasks, refresh, listen, enqueue, pause, resume, cancel, remove, retry }
})
