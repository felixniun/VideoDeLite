<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { call } from '../services/wails'
import { t } from '../i18n'
import type { AppInfo, UpdateCheck } from '../types'

const info = ref<AppInfo | null>(null)
const update = ref<UpdateCheck | null>(null)

onMounted(async () => {
  try { info.value = await call<AppInfo>('AppInfo') } catch { /* browser dev */ }
})

async function checkUpdates() {
  try { update.value = await call<UpdateCheck>('CheckForUpdates') } catch { /* noop */ }
}
</script>

<template>
  <div class="about">
    <div class="hero card">
      <img src="/appicon.png" alt="VideoDelite" class="logo" />
      <h1>{{ t('about.title') }}</h1>
      <p class="tagline">{{ t('about.tagline') }}</p>
      <div class="ver">
        {{ t('about.version') }} {{ info?.version ?? '—' }}
        <span class="dot">·</span>
        {{ t('about.ffmpeg') }} {{ info?.ffmpegOk ? '✓' : '✗' }}
      </div>
      <button class="btn btn-secondary btn-sm" @click="checkUpdates">{{ t('about.update') }}</button>
      <div v-if="update" class="update-note" :class="{ ok: update.upToDate }">
        {{ update.upToDate ? '✓ ' + t('about.upToDate') : update.note }}
      </div>
    </div>

    <section class="card sec">
      <h2>{{ t('about.privacy') }}</h2>
      <p>{{ t('about.privacyText') }}</p>
    </section>

    <section class="card sec">
      <h2>{{ t('about.license') }}</h2>
      <p>{{ t('about.licenseText') }}</p>
    </section>
  </div>
</template>

<style scoped>
.about { max-width: 640px; margin: 0 auto; }

.hero {
  text-align: center;
  padding: 40px 24px;
  margin-bottom: 16px;
}

.logo {
  width: 88px;
  height: 88px;
  border-radius: 22px;
  box-shadow: var(--shadow-float);
  margin-bottom: 16px;
}

.hero h1 {
  font-size: 22px;
  font-weight: 700;
  letter-spacing: -0.3px;
}

.tagline {
  font-size: 13.5px;
  color: var(--text-secondary);
  margin: 6px 0 12px;
}

.ver {
  font-size: 12.5px;
  color: var(--text-secondary);
  margin-bottom: 16px;
}

.dot { margin: 0 6px; }

.update-note {
  margin-top: 12px;
  font-size: 12.5px;
  color: var(--text-secondary);
}

.update-note.ok { color: var(--green); font-weight: 600; }

.sec { padding: 18px 22px; margin-bottom: 16px; }

.sec h2 {
  font-size: 13px;
  font-weight: 650;
  color: var(--text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.4px;
  margin-bottom: 8px;
}

.sec p {
  font-size: 13px;
  line-height: 1.6;
  color: var(--text);
}
</style>
