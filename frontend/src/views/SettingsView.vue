<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useAppStore } from '../stores/app'
import { call } from '../services/wails'
import { t } from '../i18n'
import type { AuthSnapshot, LogsUsage, Settings } from '../types'

const app = useAppStore()
const draft = ref<Settings | null>(null)
const saved = ref(false)
const usage = ref<LogsUsage | null>(null)

// account section state
const authState = ref<AuthSnapshot | null>(null)
const acctMode = ref<'login' | 'register' | 'verify'>('login')
const acctEmail = ref('')
const acctPassword = ref('')
const acctUsername = ref('')
const acctInvite = ref('')
const acctVerifyCode = ref('')
const acctBusy = ref(false)
const acctMsg = ref('')
const acctErr = ref('')

onMounted(async () => {
  draft.value = app.settings
    ? { ...app.settings }
    : await call<Settings>('GetSettings').catch(() => null)
  try { usage.value = await call<LogsUsage>('LogsUsage') } catch { /* noop */ }
  refreshAuth()
})

async function refreshAuth() {
  try { authState.value = await call<AuthSnapshot>('GetAuthState') } catch { authState.value = null }
}

async function accountLogin() {
  acctBusy.value = true; acctErr.value = ''; acctMsg.value = ''
  try {
    // Persist the (possibly edited) API address before hitting the server.
    if (draft.value) await app.saveSettings({ ...draft.value })
    const snap = await call<AuthSnapshot>('AccountLogin', acctEmail.value.trim(), acctPassword.value)
    authState.value = snap
    acctPassword.value = ''
    if (snap.state === 'AUTHORIZED' || snap.state === 'OFFLINE_AUTHORIZED') {
      acctMsg.value = '✓'
    } else {
      acctMode.value = 'verify'
    }
  } catch (e: any) {
    acctErr.value = cleanErr(e)
  } finally { acctBusy.value = false }
}

async function accountRegister() {
  acctBusy.value = true; acctErr.value = ''; acctMsg.value = ''
  try {
    // Persist the (possibly edited) API address before hitting the server.
    if (draft.value) await app.saveSettings({ ...draft.value })
    const devCode = await call<string>('AccountRegister', acctUsername.value.trim(), acctEmail.value.trim(), acctPassword.value, acctInvite.value.trim())
    if (devCode) acctVerifyCode.value = devCode // dev convenience
    acctMode.value = 'verify'
    acctMsg.value = t('account.verifyHint')
  } catch (e: any) {
    acctErr.value = cleanErr(e)
  } finally { acctBusy.value = false }
}

async function accountVerify() {
  acctBusy.value = true; acctErr.value = ''
  try {
    await call('AccountVerifyEmail', acctEmail.value.trim(), acctVerifyCode.value.trim())
    acctMsg.value = ''
    acctMode.value = 'login'
    await accountLogin()
  } catch (e: any) {
    acctErr.value = cleanErr(e)
  } finally { acctBusy.value = false }
}

async function accountLogout() {
  acctBusy.value = true
  try {
    authState.value = await call<AuthSnapshot>('AccountLogout')
    acctMode.value = 'login'
  } catch { /* noop */ } finally { acctBusy.value = false }
}

async function accountDelete() {
  const pw = prompt(t('account.deleteConfirm'))
  if (!pw) return
  acctBusy.value = true; acctErr.value = ''
  try {
    await call('AccountDelete', pw)
    authState.value = await call<AuthSnapshot>('GetAuthState')
    acctMode.value = 'login'
  } catch (e: any) {
    acctErr.value = cleanErr(e)
  } finally { acctBusy.value = false }
}

function cleanErr(e: any): string {
  const s = String(e?.message ?? e)
  return s.replace(/^.*Error: /, '')
}

async function pickOutput() {
  try {
    const dir = await call<string>('PickFolder')
    if (dir && draft.value) draft.value.outputDir = dir
  } catch { /* canceled */ }
}

async function save() {
  if (!draft.value) return
  await app.saveSettings({ ...draft.value })
  saved.value = true
  setTimeout(() => (saved.value = false), 1500)
}

async function viewLogs() {
  try { await call('OpenLogFolder') } catch { /* noop */ }
}

async function exportLogs() {
  try {
    const p = await call<string>('ExportLogs')
    if (p) await call('RevealPath', p)
  } catch { /* noop */ }
}

async function clearHistory() {
  if (!confirm(t('history.clearConfirm'))) return
  try { await call('ClearHistory') } catch { /* noop */ }
}

function fmtSize(bytes: number): string {
  if (bytes >= 1024 ** 2) return (bytes / 1024 ** 2).toFixed(1) + ' MB'
  if (bytes >= 1024) return (bytes / 1024).toFixed(0) + ' KB'
  return bytes + ' B'
}
</script>

<template>
  <div class="settings" v-if="draft">
    <h1 class="page-title">{{ t('settings.title') }}</h1>

    <section class="card sec">
      <h2>{{ t('settings.general') }}</h2>

      <div class="row">
        <div class="row-label">{{ t('settings.language') }}</div>
        <div class="segmented">
          <button :class="{ active: draft.language === '' }" @click="draft.language = ''">{{ t('settings.themeSystem') }}</button>
          <button :class="{ active: draft.language === 'zh-CN' }" @click="draft.language = 'zh-CN'; app.locale = 'zh-CN'">简体中文</button>
          <button :class="{ active: draft.language === 'en' }" @click="draft.language = 'en'; app.locale = 'en'">English</button>
        </div>
      </div>

      <div class="row">
        <div class="row-label">{{ t('settings.theme') }}</div>
        <div class="segmented">
          <button :class="{ active: draft.theme === '' }" @click="draft.theme = ''; app.refreshTheme()">{{ t('settings.themeSystem') }}</button>
          <button :class="{ active: draft.theme === 'light' }" @click="draft.theme = 'light'; app.applyTheme('light')">{{ t('settings.themeLight') }}</button>
          <button :class="{ active: draft.theme === 'dark' }" @click="draft.theme = 'dark'; app.applyTheme('dark')">{{ t('settings.themeDark') }}</button>
        </div>
      </div>

      <div class="row">
        <div class="row-label">
          {{ t('settings.outputDir') }}
          <div class="row-hint">{{ t('settings.outputDirHint') }}</div>
        </div>
        <div class="inline-input">
          <input type="text" v-model="draft.outputDir" spellcheck="false" />
          <button class="btn btn-secondary btn-sm" @click="pickOutput()">…</button>
        </div>
      </div>

      <div class="row">
        <div class="row-label">{{ t('settings.conflict') }}</div>
        <div class="segmented">
          <button :class="{ active: draft.conflictStrategy === 'auto' }" @click="draft.conflictStrategy = 'auto'">{{ t('settings.conflict.auto') }}</button>
          <button :class="{ active: draft.conflictStrategy === 'overwrite' }" @click="draft.conflictStrategy = 'overwrite'">{{ t('settings.conflict.overwrite') }}</button>
          <button :class="{ active: draft.conflictStrategy === 'skip' }" @click="draft.conflictStrategy = 'skip'">{{ t('settings.conflict.skip') }}</button>
        </div>
      </div>
    </section>

    <section class="card sec">
      <h2>{{ t('settings.encoding') }}</h2>

      <div class="row">
        <div class="row-label">{{ t('settings.defaultCodec') }}</div>
        <div class="segmented">
          <button :class="{ active: draft.defaultCodec === 'h264' }" @click="draft.defaultCodec = 'h264'">H.264</button>
          <button :class="{ active: draft.defaultCodec === 'h265' }" @click="draft.defaultCodec = 'h265'">H.265</button>
        </div>
      </div>

      <div class="row">
        <div class="row-label">
          {{ t('settings.hardware') }}
          <div class="row-hint">{{ t('settings.hardwareHint') }}</div>
        </div>
        <div class="switch" :class="{ on: draft.hardwareEncode }" @click="draft.hardwareEncode = !draft.hardwareEncode"></div>
      </div>

      <div class="row">
        <div class="row-label">{{ t('settings.paths') }}</div>
        <div class="paths">
          <input type="text" v-model="draft.ffmpegPath" :placeholder="t('settings.ffmpegPath')" spellcheck="false" />
          <input type="text" v-model="draft.ffprobePath" :placeholder="t('settings.ffprobePath')" spellcheck="false" />
        </div>
      </div>
    </section>

    <section class="card sec">
      <h2>{{ t('settings.account') }}</h2>

      <!-- API address is fixed to the domain endpoint (hidden from UI) -->

      <div class="row">
        <div class="row-label">{{ t('account.state.' + (authState?.state ?? 'UNAUTHENTICATED')) }}</div>
        <span
          class="badge"
          :class="authState?.canUseProfessional ? 'badge-green' : 'badge-gray'"
        >{{ authState?.canUseProfessional ? '✓' : '–' }}</span>
      </div>

      <template v-if="!authState?.canUseProfessional">
        <div v-if="acctMode !== 'verify'" class="acct-form">
          <input type="text" v-model="acctEmail" :placeholder="t('account.email')" spellcheck="false" />
          <input type="password" v-model="acctPassword" :placeholder="t('account.password')" />
          <template v-if="acctMode === 'register'">
            <input type="text" v-model="acctUsername" :placeholder="t('account.username')" spellcheck="false" />
            <input type="text" v-model="acctInvite" :placeholder="t('account.inviteCode')" spellcheck="false" />
          </template>
          <div class="acct-actions">
            <button v-if="acctMode === 'login'" class="btn btn-primary btn-sm" :disabled="acctBusy || !acctEmail || !acctPassword" @click="accountLogin">{{ t('account.login') }}</button>
            <button v-else class="btn btn-primary btn-sm" :disabled="acctBusy || !acctEmail || !acctUsername || !acctPassword" @click="accountRegister">{{ t('account.registerBtn') }}</button>
            <button class="btn btn-ghost btn-sm" @click="acctMode = acctMode === 'login' ? 'register' : 'login'">
              {{ acctMode === 'login' ? t('account.switchToRegister') : t('account.switchToLogin') }}
            </button>
          </div>
        </div>

        <div v-else class="acct-form">
          <div class="row-hint">{{ acctMsg || t('account.verifyHint') }}</div>
          <input type="text" v-model="acctVerifyCode" :placeholder="t('account.verifyBtn')" spellcheck="false" />
          <div class="acct-actions">
            <button class="btn btn-primary btn-sm" :disabled="acctBusy || !acctVerifyCode" @click="accountVerify">{{ t('account.verifyBtn') }}</button>
          </div>
        </div>
      </template>

      <template v-else>
        <div class="acct-actions">
          <button class="btn btn-secondary btn-sm" :disabled="acctBusy" @click="accountLogout">{{ t('account.logout') }}</button>
          <button class="btn btn-ghost btn-sm acct-danger" :disabled="acctBusy" @click="accountDelete">{{ t('account.delete') }}</button>
        </div>
        <div class="row-hint">{{ t('account.deleteHint') }}</div>
      </template>

      <div v-if="acctErr" class="error-box">{{ acctErr }}</div>
    </section>

    <section class="card sec">
      <h2>{{ t('settings.logs') }}</h2>
      <div class="row">
        <div class="row-label">
          {{ t('settings.viewLogs') }}
          <div class="row-hint" v-if="usage">{{ t('settings.logUsage', { count: usage.fileCount, size: fmtSize(usage.totalBytes) }) }}</div>
        </div>
        <div class="btn-col">
          <button class="btn btn-secondary btn-sm" @click="viewLogs">{{ t('settings.viewLogs') }}</button>
          <button class="btn btn-secondary btn-sm" @click="exportLogs">{{ t('settings.exportLogs') }}</button>
        </div>
      </div>
    </section>

    <section class="card sec">
      <h2>{{ t('settings.data') }}</h2>
      <div class="row">
        <div class="row-label">{{ t('settings.clearHistory') }}</div>
        <button class="btn btn-secondary btn-sm" @click="clearHistory">{{ t('settings.clearHistory') }}</button>
      </div>
    </section>

    <div class="save-row">
      <button class="btn btn-primary" @click="save">{{ saved ? '✓ ' + t('settings.saved') : t('settings.save') }}</button>
    </div>
  </div>
</template>

<style scoped>
.settings { max-width: 720px; margin: 0 auto; }

.page-title {
  font-size: 26px;
  font-weight: 700;
  letter-spacing: -0.3px;
  margin-bottom: 18px;
}

.sec { padding: 18px 22px; margin-bottom: 16px; }

.sec h2 {
  font-size: 13px;
  font-weight: 650;
  color: var(--text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.4px;
  margin-bottom: 14px;
}

.row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  padding: 10px 0;
  border-bottom: 1px solid var(--separator);
  min-height: 44px;
}

.row:last-child { border-bottom: none; }

.row-label { font-size: 13.5px; font-weight: 570; }

.row-hint {
  font-size: 12px;
  font-weight: 400;
  color: var(--text-secondary);
  margin-top: 2px;
}

.inline-input {
  display: flex;
  gap: 8px;
  flex: 1;
  max-width: 480px;
}

.inline-input input { flex: 1; font-size: 13px; }

.paths {
  display: flex;
  flex-direction: column;
  gap: 8px;
  flex: 1;
  max-width: 480px;
}

.paths input { font-size: 12.5px; }

.btn-col {
  display: flex;
  flex-direction: column;
  gap: 8px;
  align-items: flex-end;
}

/* account form */
.acct-form {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 12px 0 4px;
}

.acct-form input { font-size: 13px; width: 100%; }

.acct-actions {
  display: flex;
  gap: 10px;
  align-items: center;
}

.acct-danger { color: var(--red); }

.acct-danger:hover { background: rgba(255, 69, 58, 0.1); }

.error-box {
  margin-top: 12px;
  padding: 10px 16px;
  border-radius: var(--radius-md);
  background: rgba(255, 69, 58, 0.12);
  color: var(--red);
  font-size: 12.5px;
  word-break: break-all;
}

.save-row {
  display: flex;
  justify-content: flex-end;
  margin-top: 4px;
}
</style>
