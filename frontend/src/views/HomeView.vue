<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { call } from '../services/wails'
import { useAppStore } from '../stores/app'
import { useTaskStore } from '../stores/tasks'
import { t } from '../i18n'
import type { AuthSnapshot, EncoderOption, MediaInfo } from '../types'

const app = useAppStore()
const tasks = useTaskStore()

const files = ref<MediaInfo[]>([])
const scanning = ref(false)
const dragOver = ref(false)
const error = ref('')

const mode = ref<'simple' | 'professional'>('simple')
const auth = ref<AuthSnapshot | null>(null)

const codec = ref('h264')
const quality = ref('mid')
const container = ref('mp4')
const encoderSel = ref('auto')
const outputDir = ref('')

// ---- professional state (plan §63) ----
const proEncoder = ref('auto')
const encoderOptions = ref<EncoderOption[]>([])
const rateControl = ref<'cbr' | 'vbr' | 'cq'>('vbr')
const bitrate = ref(8)
const maxBitrate = ref(12)
const qualityCQ = ref(23)
const preset = ref('medium')
const audioBitrate = ref(128)
const audioRC = ref<'cbr' | 'vbr'>('cbr')
const audioQuality = ref(3)
const preserveMetadata = ref(true)
const preserveSubtitles = ref(true)
const preserveChapters = ref(true)
const preserveAudioTracks = ref(true)

const audioBitrateChoices = [64, 96, 128, 160, 192, 224, 256, 320, 384, 512]

const presetsFor = computed(() => {
  if (proEncoder.value === 'nvenc' || proEncoder.value === 'cpu') {
    return [
        { v: 'fast', l: t('home.presetFast') },
        { v: 'medium', l: t('home.presetMedium') },
        { v: 'slow', l: t('home.presetSlow') }
      ]
  }
  return [{ v: 'medium', l: t('home.presetMedium') }]
})

watch(codec, refreshEncoders)
watch(proEncoder, () => { /* availability comes pre-resolved */ })

async function refreshEncoders() {
  try {
    encoderOptions.value = await call<EncoderOption[]>('GetEncoderOptions', codec.value)
  } catch {
    encoderOptions.value = []
  }
  if (!encoderOptions.value.find((o) => o.id === proEncoder.value && o.available)) {
    proEncoder.value = 'auto'
  }
}

const proLocked = computed(() => auth.value !== null && !auth.value.canUseProfessional)

async function loadAuth() {
  try { auth.value = await call<AuthSnapshot>('GetAuthState') } catch { auth.value = null }
}
loadAuth()

const proError = computed(() => {
  if (rateControl.value === 'vbr' && maxBitrate.value < bitrate.value) {
    return t('home.proMaxLtAvg')
  }
  return ''
})

// scheduling (professional only; simple mode is always sequential, plan §35)
const parallel = ref<1 | 2 | 3>(1)

// estimated output size for professional mode: Σ duration × bitrate / 8
const proEstimateMB = computed(() => {
  if (rateControl.value === 'cq') return 0 // encoder decides, cannot estimate
  const mbps = rateControl.value === 'vbr' ? bitrate.value : bitrate.value
  const audio = audioRC.value === 'cbr' ? audioBitrate.value / 1000 : 0.128
  return files.value.reduce((s, f) => s + (f.duration * (mbps + audio)) / 8, 0)
})

function fmtMB(mb: number): string {
  return mb >= 1024 ? (mb / 1024).toFixed(2) + ' GB' : Math.round(mb) + ' MB'
}

const totalSize = computed(() => files.value.reduce((s, f) => s + f.fileSize, 0))
const first = computed(() => files.value[0])

const estBitrate = computed(() => {
  if (!first.value?.video) return 0
  // Preview via the Go table so UI and encoder share one source of truth.
  return first.value.video ? 0 : 0 // replaced below by Go call
})

async function estimate() {
  if (!first.value?.video) return 0
  try {
    return await call<number>('EstimateSimpleBitrate',
      first.value.video.width, first.value.video.height, first.value.video.fps,
      codec.value, quality.value)
  } catch {
    return 0
  }
}

const targetBitrate = ref(0)
async function refreshEstimate() {
  targetBitrate.value = await estimate()
}

async function startProfessional() {
  if (files.value.length === 0 || proError.value) return
  error.value = ''
  try {
    // Scheduling is a professional-mode option (owner-approved §35 extension).
    await call('SetTaskConcurrency', parallel.value)
    for (const f of files.value) {
      await call('CreateProfessionalTask', {
        inputPath: f.filePath,
        mode: 'professional',
        outputDir: outputDir.value,
        encoder: proEncoder.value,
        config: {
          container: container.value,
          videoCodec: codec.value,
          encoder: proEncoder.value,
          rateControl: rateControl.value,
          bitrateMbps: bitrate.value,
          maxBitrateMbps: rateControl.value === 'vbr' ? maxBitrate.value : bitrate.value,
          quality: qualityCQ.value,
          audioCodec: 'aac',
          audioBitrateKbps: audioBitrate.value,
          audioRateControl: audioRC.value,
          audioQuality: audioQuality.value,
          preset: preset.value,
          preserveMetadata: preserveMetadata.value,
          preserveSubtitles: preserveSubtitles.value,
          preserveChapters: preserveChapters.value,
          preserveAudioTracks: preserveAudioTracks.value
        }
      })
    }
    files.value = []
  } catch (e: any) {
    error.value = String(e?.message ?? e)
  }
}

async function importPaths(paths: string[]) {
  if (paths.length === 0) return
  error.value = ''
  scanning.value = true
  try {
    const expanded = await call<string[]>('ScanPaths', paths)
    const infos = await call<MediaInfo[]>('AnalyzeFiles', expanded)
    for (const info of infos) {
      if (!files.value.find((f) => f.filePath === info.filePath)) {
        files.value.push(info)
      }
    }
    outputDir.value = outputDir.value || (app.settings?.outputDir ?? '')
    refreshEstimate()
  } catch (e: any) {
    error.value = String(e?.message ?? e)
  } finally {
    scanning.value = false
  }
}

async function browse() {
  try {
    const picked = await call<string[]>('PickVideoFiles')
    await importPaths(picked)
  } catch { /* canceled */ }
}

async function pickOutput() {
  try {
    const dir = await call<string>('PickFolder')
    if (dir) outputDir.value = dir
  } catch { /* canceled */ }
}

function remove(idx: number) {
  files.value.splice(idx, 1)
  refreshEstimate()
}

function clearAll() {
  files.value = []
  error.value = ''
}

function onDrop(e: DragEvent) {
  dragOver.value = false
  const paths: string[] = []
  for (const item of e.dataTransfer?.files ?? []) {
    // Wails maps webkitRelativePath/name only; use the Go-valid full path trick:
    // Wails v2 provides file.fullPath in the dataTransfer under the "file" protocol.
    const p = (item as any).path ?? (item as any).fullPath ?? item.name
    paths.push(String(p).replace(/^file:\/\//, ''))
  }
  importPaths(paths)
}

async function start() {
  if (files.value.length === 0) return
  error.value = ''
  try {
    // Simple mode is frozen to sequential scheduling (plan §35).
    await call('SetTaskConcurrency', 1)
    await tasks.enqueue(
      files.value.map((f) => f.filePath),
      {
        codec: codec.value,
        quality: quality.value,
        container: container.value,
        outputDir: outputDir.value,
        encoder: encoderSel.value
      }
    )
    files.value = []
  } catch (e: any) {
    error.value = String(e?.message ?? e)
  }
}

function fmtSize(bytes: number): string {
  if (bytes >= 1024 ** 3) return (bytes / 1024 ** 3).toFixed(2) + ' GB'
  if (bytes >= 1024 ** 2) return (bytes / 1024 ** 2).toFixed(1) + ' MB'
  return Math.max(1, Math.round(bytes / 1024)) + ' KB'
}

function fmtDuration(sec: number): string {
  const m = Math.floor(sec / 60)
  const s = Math.round(sec % 60)
  return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
}
</script>

<template>
  <div class="home">
    <h1 class="page-title">{{ t('nav.home') }}</h1>

    <!-- drop zone -->
    <div
      class="dropzone"
      :class="{ over: dragOver, scanning }"
      @click="browse()"
      @dragover.prevent="dragOver = true"
      @dragleave="dragOver = false"
      @drop.prevent="onDrop"
    >
      <div class="dz-icon">🎬</div>
      <div class="dz-title">{{ scanning ? t('home.scanning') : t('home.drop') }}</div>
      <div class="dz-hint">{{ t('home.dropHint') }}</div>
    </div>

    <div v-if="error" class="error-box">{{ error }}</div>

    <!-- file list -->
    <div v-if="files.length" class="filelist card">
      <div class="fl-head">
        <span>{{ t('home.selected', { n: files.length }) }} · {{ t('home.totalSize', { size: fmtSize(totalSize) }) }}</span>
        <button class="btn btn-ghost btn-sm" @click="clearAll">{{ t('home.clear') }}</button>
      </div>
      <div class="fl-scroll">
        <div v-for="(f, i) in files" :key="f.filePath" class="fl-row">
          <div class="fl-info">
            <div class="fl-name" :title="f.filePath">{{ f.fileName }}</div>
            <div class="fl-meta">
              <template v-if="f.video">
                {{ f.video.width }}×{{ f.video.height }} ·
                {{ f.video.codec.toUpperCase() }} ·
                {{ fmtDuration(f.duration) }} ·
                <span v-if="f.video.hdr" class="hdr-tag">{{ f.video.hdr }}</span>
                <span v-if="f.video.bitDepth >= 10">10-bit</span>
                {{ fmtSize(f.fileSize) }}
              </template>
            </div>
          </div>
          <button class="btn btn-secondary btn-sm" @click.stop="remove(i)">{{ t('home.remove') }}</button>
        </div>
      </div>
    </div>

    <!-- mode switch -->
    <div class="mode-row">
      <div class="segmented mode-seg">
        <button :class="{ active: mode === 'simple' }" @click="mode = 'simple'">{{ t('home.modeSimple') }}</button>
        <button :class="{ active: mode === 'professional' }" @click="mode = 'professional'; loadAuth(); refreshEncoders()">{{ t('home.modeProfessional') }}</button>
      </div>
      <span v-if="proLocked && mode === 'professional'" class="badge badge-orange">🔒 {{ t('home.proLocked') }}</span>
    </div>

    <!-- simple mode config -->
    <div v-if="files.length && mode === 'simple'" class="config card">
      <div class="cfg-grid">
        <div class="cfg-item">
          <div class="cfg-label">{{ t('home.codec') }}</div>
          <div class="segmented">
            <button :class="{ active: codec === 'h264' }" @click="codec = 'h264'; refreshEstimate()">H.264</button>
            <button :class="{ active: codec === 'h265' }" @click="codec = 'h265'; refreshEstimate()">H.265</button>
          </div>
        </div>

        <div class="cfg-item">
          <div class="cfg-label">{{ t('home.quality') }}</div>
          <div class="segmented">
            <button :class="{ active: quality === 'low' }" @click="quality = 'low'; refreshEstimate()">{{ t('quality.lowShort') }}</button>
            <button :class="{ active: quality === 'mid' }" @click="quality = 'mid'; refreshEstimate()">{{ t('quality.midShort') }}</button>
            <button :class="{ active: quality === 'high' }" @click="quality = 'high'; refreshEstimate()">{{ t('quality.highShort') }}</button>
          </div>
          <div class="quality-desc">{{ t('quality.desc.' + quality) }}</div>
        </div>

        <div class="cfg-item">
          <div class="cfg-label">{{ t('home.container') }}</div>
          <div class="segmented">
            <button :class="{ active: container === 'mp4' }" @click="container = 'mp4'">MP4</button>
            <button :class="{ active: container === 'mkv' }" @click="container = 'mkv'">MKV</button>
          </div>
        </div>

        <div class="cfg-item">
          <div class="cfg-label">{{ t('home.encoder') }}</div>
          <div class="segmented">
            <button :class="{ active: encoderSel === 'auto' }" @click="encoderSel = 'auto'">{{ t('home.encoderAuto') }}</button>
            <button :class="{ active: encoderSel === 'cpu' }" @click="encoderSel = 'cpu'">CPU</button>
          </div>
        </div>
      </div>

      <div class="cfg-output">
        <div class="cfg-label">{{ t('home.output') }}</div>
        <div class="output-row">
          <input type="text" v-model="outputDir" class="output-input" spellcheck="false" />
          <button class="btn btn-secondary btn-sm" @click="pickOutput()">…</button>
        </div>
        <div v-if="targetBitrate > 0" class="output-hint">
          {{ t('home.estBitrate', { rate: targetBitrate.toFixed(1) + ' Mbps' }) }}
          <template v-if="first?.video"> · {{ first.video.width }}×{{ first.video.height }}@{{ first.video.fps.toFixed(0) }}fps</template>
        </div>
      </div>

      <button class="btn btn-primary start-btn" @click="start">
        {{ t('home.startN', { n: files.length }) }}
      </button>
    </div>

    <!-- professional mode config (plan §63) -->
    <div v-if="files.length && mode === 'professional'" class="config card pro">
      <div v-if="proLocked" class="pro-locked">
        🔒 {{ t('home.proLocked') }}
        <div v-if="auth?.reason" class="pro-locked-reason">{{ auth.reason }}</div>
      </div>

      <template v-else>
        <div class="pro-sec">
          <div class="pro-sec-title">{{ t('home.proVideo') }}</div>
          <div class="pro-grid">
            <label class="pro-field">
              <span>{{ t('home.proCodec') }}</span>
              <div class="segmented">
                <button :class="{ active: codec === 'h264' }" @click="codec = 'h264'">H.264</button>
                <button :class="{ active: codec === 'h265' }" @click="codec = 'h265'">H.265</button>
              </div>
            </label>

            <label class="pro-field">
              <span>{{ t('home.proEncoder') }}</span>
              <select v-model="proEncoder">
                <option v-for="o in encoderOptions" :key="o.id" :value="o.id" :disabled="!o.available">
                  {{ o.label }}{{ o.available ? '' : ' — ' + (o.reason || 'unavailable') }}
                </option>
              </select>
            </label>

            <label class="pro-field">
              <span>{{ t('home.proRateControl') }}</span>
              <div class="segmented">
                <button :class="{ active: rateControl === 'cbr' }" @click="rateControl = 'cbr'">CBR</button>
                <button :class="{ active: rateControl === 'vbr' }" @click="rateControl = 'vbr'">VBR</button>
                <button :class="{ active: rateControl === 'cq' }" @click="rateControl = 'cq'">Quality</button>
              </div>
            </label>

            <label v-if="rateControl === 'cbr' || rateControl === 'vbr'" class="pro-field">
              <span>{{ t('home.proBitrate') }}</span>
              <input type="number" v-model.number="bitrate" min="0.5" max="120" step="0.5" />
            </label>

            <label v-if="rateControl === 'vbr'" class="pro-field">
              <span>{{ t('home.proMaxBitrate') }}</span>
              <input type="number" v-model.number="maxBitrate" min="0.5" max="120" step="0.5" />
            </label>

            <label v-if="rateControl === 'cq'" class="pro-field">
              <span>{{ t('home.proQuality') }}</span>
              <input type="number" v-model.number="qualityCQ" min="0" max="51" />
            </label>

            <label class="pro-field">
              <span>{{ t('home.proPreset') }}</span>
              <select v-model="preset">
                <option v-for="p in presetsFor" :key="p.v" :value="p.v">{{ p.l }}</option>
              </select>
            </label>
          </div>
        </div>

        <div class="pro-sec">
          <div class="pro-sec-title">{{ t('home.proAudio') }}</div>
          <div class="pro-grid">
            <label class="pro-field">
              <span>{{ t('home.proRateControl') }}</span>
              <div class="segmented">
                <button :class="{ active: audioRC === 'cbr' }" @click="audioRC = 'cbr'">CBR</button>
                <button :class="{ active: audioRC === 'vbr' }" @click="audioRC = 'vbr'">VBR</button>
              </div>
            </label>

            <label v-if="audioRC === 'cbr'" class="pro-field">
              <span>{{ t('home.proAudioBitrate') }}</span>
              <select v-model.number="audioBitrate">
                <option v-for="b in audioBitrateChoices" :key="b" :value="b">{{ b }} kbps</option>
              </select>
            </label>

            <label v-else class="pro-field">
              <span>{{ t('home.proAudioQuality') }}</span>
              <input type="range" v-model.number="audioQuality" min="1" max="5" step="1" class="range" />
              <span class="range-val">{{ audioQuality }}</span>
            </label>
          </div>
        </div>

        <div class="pro-sec">
          <div class="pro-sec-title">{{ t('home.proStreams') }}</div>
          <div class="pro-toggles">
            <div class="toggle-row">
              <span>{{ t('home.proKeepAudio') }}</span>
              <div class="switch" :class="{ on: preserveAudioTracks }" @click="preserveAudioTracks = !preserveAudioTracks"></div>
            </div>
            <div class="toggle-row">
              <span>{{ t('home.proKeepSubs') }}</span>
              <div class="switch" :class="{ on: preserveSubtitles }" @click="preserveSubtitles = !preserveSubtitles"></div>
            </div>
            <div class="toggle-row">
              <span>{{ t('home.proKeepChapters') }}</span>
              <div class="switch" :class="{ on: preserveChapters }" @click="preserveChapters = !preserveChapters"></div>
            </div>
            <div class="toggle-row">
              <span>{{ t('home.proKeepMeta') }}</span>
              <div class="switch" :class="{ on: preserveMetadata }" @click="preserveMetadata = !preserveMetadata"></div>
            </div>
          </div>
        </div>

        <div class="pro-sec">
          <div class="pro-sec-title">{{ t('home.proOutput') }}</div>
          <div class="pro-grid">
            <label class="pro-field">
              <span>{{ t('home.container') }}</span>
              <div class="segmented">
                <button :class="{ active: container === 'mp4' }" @click="container = 'mp4'">MP4</button>
                <button :class="{ active: container === 'mkv' }" @click="container = 'mkv'">MKV</button>
              </div>
            </label>

            <label class="pro-field">
              <span>{{ t('home.parallel') }}</span>
              <div class="segmented">
                <button :class="{ active: parallel === 1 }" @click="parallel = 1">{{ t('home.parallelSeq') }}</button>
                <button :class="{ active: parallel === 2 }" @click="parallel = 2">{{ t('home.parallel2') }}</button>
                <button :class="{ active: parallel === 3 }" @click="parallel = 3">{{ t('home.parallel3') }}</button>
              </div>
              <div class="quality-desc">{{ t('home.parallelHint') }}</div>
            </label>

            <label class="pro-field">
              <span>{{ t('home.output') }}</span>
              <div class="output-row">
                <input type="text" v-model="outputDir" class="output-input" spellcheck="false" />
                <button class="btn btn-secondary btn-sm" @click="pickOutput()">…</button>
              </div>
            </label>
          </div>

          <div v-if="proEstimateMB > 0" class="output-hint" style="margin-top: 10px">
            {{ t('home.estSize', { size: fmtMB(proEstimateMB), rate: bitrate + ' Mbps' }) }}
          </div>
          <div v-else-if="rateControl === 'cq'" class="output-hint" style="margin-top: 10px">
            {{ t('home.estSizeCQ') }}
          </div>
        </div>

        <div v-if="proError" class="error-box">{{ proError }}</div>

        <button class="btn btn-primary start-btn" :disabled="!!proError" @click="startProfessional">
          {{ t('home.startN', { n: files.length }) }}
        </button>
      </template>
    </div>
  </div>
</template>

<style scoped>
.home { max-width: 860px; margin: 0 auto; }

.page-title {
  font-size: 26px;
  font-weight: 700;
  letter-spacing: -0.3px;
  margin-bottom: 18px;
}

.dropzone {
  background: var(--card);
  border: 2px dashed var(--separator);
  border-radius: var(--radius-xl);
  box-shadow: var(--shadow-card);
  padding: 48px 24px;
  text-align: center;
  cursor: pointer;
  transition: all 0.2s ease;
}

.dropzone:hover,
.dropzone.over {
  border-color: var(--accent);
  background: var(--accent-soft);
}

.dz-icon { font-size: 40px; margin-bottom: 10px; }
.dz-title { font-size: 16px; font-weight: 600; margin-bottom: 4px; }
.dz-hint { font-size: 12.5px; color: var(--text-secondary); }

.error-box {
  margin-top: 12px;
  padding: 10px 16px;
  border-radius: var(--radius-md);
  background: rgba(255, 69, 58, 0.12);
  color: var(--red);
  font-size: 13px;
}

.filelist {
  margin-top: 16px;
  padding: 14px 18px;
}

.fl-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 13px;
  color: var(--text-secondary);
  font-weight: 600;
  margin-bottom: 8px;
}

.fl-scroll { max-height: 220px; overflow-y: auto; }

.fl-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 9px 0;
  border-bottom: 1px solid var(--separator);
}

.fl-row:last-child { border-bottom: none; }

.fl-name {
  font-size: 13.5px;
  font-weight: 590;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 480px;
}

.fl-meta {
  font-size: 12px;
  color: var(--text-secondary);
  margin-top: 2px;
}

.hdr-tag {
  color: var(--orange);
  font-weight: 700;
  margin-right: 4px;
}

.config {
  margin-top: 16px;
  padding: 20px 22px;
}

.cfg-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 16px;
}

.cfg-item { display: flex; flex-direction: column; gap: 8px; }

.cfg-label {
  font-size: 12px;
  font-weight: 650;
  color: var(--text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.4px;
}

.cfg-output { margin-top: 18px; display: flex; flex-direction: column; gap: 8px; }

.output-row { display: flex; gap: 8px; }

.output-input { flex: 1; font-size: 13px; }

.output-hint {
  font-size: 12px;
  color: var(--text-secondary);
}

.quality-desc {
  font-size: 12px;
  color: var(--text-tertiary);
  margin-top: 2px;
}

.start-btn {
  margin-top: 20px;
  width: 100%;
  padding: 12px;
  font-size: 15px;
}

/* mode switch row */
.mode-row {
  display: flex;
  align-items: center;
  gap: 12px;
  margin: 22px 0 18px;
}

.mode-row:first-child {
  margin-top: 0;
}

.mode-seg button { min-width: 110px; }

/* professional panel */
.pro-sec { padding: 14px 0; border-bottom: 1px solid var(--separator); }
.pro-sec:first-child { padding-top: 0; }
.pro-sec:last-of-type { border-bottom: none; }

.pro-sec-title {
  font-size: 12px;
  font-weight: 650;
  color: var(--text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.4px;
  margin-bottom: 10px;
}

.pro-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 14px 20px;
  align-items: end;
}

.pro-field {
  display: flex;
  flex-direction: column;
  gap: 6px;
  font-size: 12.5px;
  color: var(--text-secondary);
  font-weight: 570;
}

.pro-field select,
.pro-field input { font-size: 13px; width: 100%; }

.range { accent-color: var(--accent); width: 100%; }
.range-val { font-weight: 700; color: var(--text); }

.pro-toggles { display: flex; flex-direction: column; gap: 4px; }

.toggle-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 0;
  font-size: 13.5px;
  min-height: 40px;
  border-bottom: 1px solid var(--separator);
}

.toggle-row:last-child { border-bottom: none; }

.pro-locked {
  padding: 32px;
  text-align: center;
  font-size: 14px;
  color: var(--text-secondary);
}

.pro-locked-reason {
  margin-top: 8px;
  font-size: 12px;
  color: var(--text-tertiary);
}
</style>
