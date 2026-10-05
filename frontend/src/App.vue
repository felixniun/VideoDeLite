<script setup lang="ts">
import { onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { useAppStore } from './stores/app'
import { useTaskStore } from './stores/tasks'
import { t } from './i18n'
import { inWails, minimise, toggleMaximise, quit } from './services/wails'

const app = useAppStore()
const tasks = useTaskStore()
const route = useRoute()

onMounted(() => {
  app.init()
  tasks.listen()
  tasks.refresh()
})

const navItems = [
  { path: '/', key: 'nav.home', icon: 'home' },
  { path: '/tasks', key: 'nav.tasks', icon: 'tasks' },
  { path: '/history', key: 'nav.history', icon: 'history' },
  { path: '/settings', key: 'nav.settings', icon: 'settings' },
  { path: '/about', key: 'nav.about', icon: 'about' }
]

function icon(name: string): string {
  switch (name) {
    case 'home': return '⌂'
    case 'tasks': return '⚡'
    case 'history': return '🕘'
    case 'settings': return '⚙'
    case 'about': return 'ⓘ'
  }
  return ''
}

const activeCount = () => tasks.tasks.filter((x) => !['Completed', 'Failed', 'Canceled'].includes(x.state)).length
</script>

<template>
  <div class="shell">
    <!-- custom title bar (frameless window) -->
    <div class="titlebar">
      <div class="titlebar-title">
        <img src="/appicon.png" alt="" class="titlebar-logo" />
        VideoDelite
      </div>
      <div class="titlebar-spacer" style="--wails-draggable:drag"></div>
      <div class="titlebar-controls">
        <button class="tb-btn" @click="minimise()" aria-label="minimize">─</button>
        <button class="tb-btn" @click="toggleMaximise()" aria-label="maximize">□</button>
        <button class="tb-btn tb-close" @click="quit()" aria-label="close">✕</button>
      </div>
    </div>

    <div class="body">
      <aside class="sidebar">
        <router-link
          v-for="item in navItems"
          :key="item.path"
          :to="item.path"
          class="nav-item"
          :class="{ active: route.path === item.path }"
        >
          <span class="nav-icon">{{ icon(item.icon) }}</span>
          <span>{{ t(item.key) }}</span>
          <span v-if="item.path === '/tasks' && activeCount() > 0" class="nav-badge">{{ activeCount() }}</span>
        </router-link>
      </aside>

      <main class="content">
        <router-view v-slot="{ Component }">
          <transition name="view-slide" mode="out-in">
            <component :is="Component" />
          </transition>
        </router-view>
      </main>
    </div>
  </div>
</template>

<style scoped>
.shell {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.titlebar {
  display: flex;
  align-items: center;
  height: 44px;
  padding-left: 16px;
  background: var(--bg-elevated);
  border-bottom: 1px solid var(--separator);
  flex-shrink: 0;
  -webkit-app-region: drag;
}

.titlebar-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  font-weight: 600;
  color: var(--text-secondary);
}

.titlebar-logo {
  width: 18px;
  height: 18px;
  border-radius: 5px;
}

.titlebar-spacer {
  flex: 1;
  height: 100%;
}

.titlebar-controls {
  display: flex;
  height: 100%;
  -webkit-app-region: no-drag;
}

.tb-btn {
  width: 46px;
  height: 100%;
  font-size: 12px;
  color: var(--text-secondary);
  display: flex;
  align-items: center;
  justify-content: center;
  transition: background 0.15s;
}

.tb-btn:hover { background: var(--bg-inset); }
.tb-close:hover { background: var(--red); color: #fff; }

.body {
  display: flex;
  flex: 1;
  min-height: 0;
}

.sidebar {
  width: 200px;
  flex-shrink: 0;
  padding: 12px 10px;
  display: flex;
  flex-direction: column;
  gap: 2px;
  background: var(--sidebar-bg);
}

.nav-item {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 9px 14px;
  border-radius: var(--radius-md);
  color: var(--text-secondary);
  text-decoration: none;
  font-weight: 540;
  font-size: 13.5px;
  transition: all 0.15s ease;
}

.nav-item:hover { background: var(--bg-inset); }

.nav-item.active {
  background: var(--accent);
  color: #fff;
}

.nav-icon {
  width: 18px;
  text-align: center;
  font-size: 14px;
}

.nav-badge {
  margin-left: auto;
  background: var(--red);
  color: #fff;
  font-size: 10.5px;
  font-weight: 700;
  border-radius: 980px;
  padding: 1px 7px;
}

.nav-item.active .nav-badge { background: rgba(255, 255, 255, 0.3); }

.content {
  flex: 1;
  min-width: 0;
  overflow-y: auto;
  padding: 24px 28px;
}
</style>
