import { defineStore } from 'pinia'
import { ref, watch } from 'vue'
import { call } from '../services/wails'
import { getLocale, setLocale, type Locale } from '../i18n'
import type { Settings } from '../types'

export const useAppStore = defineStore('app', () => {
  const theme = ref<'light' | 'dark'>('light')
  const locale = ref<Locale>(getLocale())
  const settings = ref<Settings | null>(null)
  const loaded = ref(false)

  function applyTheme(t: 'light' | 'dark') {
    theme.value = t
    document.documentElement.classList.toggle('dark', t === 'dark')
  }

  async function init() {
    try {
      // First-run language follows the OS (plan §9); user choice persists.
      const sys = await call<string>('SystemLanguage')
      setLocale(sys as Locale)
      locale.value = sys as Locale
      const s = await call<Settings>('GetSettings')
      settings.value = s
      if (s.language === 'zh-CN' || s.language === 'en') {
        setLocale(s.language as Locale)
        locale.value = s.language as Locale
      }
      const sysTheme = await call<string>('SystemTheme')
      applyTheme((s.theme || sysTheme) as 'light' | 'dark')
      loaded.value = true
    } catch (e) {
      // Browser dev mode: keep defaults.
      applyTheme('light')
      loaded.value = true
    }
  }

  watch(locale, (l) => setLocale(l))

  async function saveSettings(next: Settings) {
    await call('SaveSettings', next)
    settings.value = next
    if (next.theme === 'light' || next.theme === 'dark') {
      applyTheme(next.theme)
    } else {
      const sysTheme = await call<string>('SystemTheme')
      applyTheme(sysTheme as 'light' | 'dark')
    }
  }

  async function refreshTheme() {
    if (settings.value?.theme) return
    const sysTheme = await call<string>('SystemTheme')
    applyTheme(sysTheme as 'light' | 'dark')
  }

  return { theme, locale, settings, loaded, init, applyTheme, saveSettings, refreshTheme }
})
