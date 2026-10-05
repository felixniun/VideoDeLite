// Shared view models — mirror of the Go binding structs.

export interface VideoStream {
  index: number
  codec: string
  profile: string
  width: number
  height: number
  fps: number
  bitrate: number
  pixelFormat: string
  bitDepth: number
  colorSpace: string
  colorTransfer: string
  colorPrimaries: string
  hdr: string
}

export interface AudioStream {
  index: number
  codec: string
  profile: string
  channels: number
  sampleRate: number
  bitrate: number
  language: string
  title: string
  default: boolean
}

export interface SubtitleStream {
  index: number
  codec: string
  language: string
  title: string
  default: boolean
}

export interface MediaInfo {
  filePath: string
  fileName: string
  fileSize: number
  format: string
  duration: number
  overallBitrate: number
  video: VideoStream | null
  audio: AudioStream[]
  subtitles: SubtitleStream[]
  chapters: number
  attachments: number
  rotation: number
  metadata: Record<string, string>
}

export interface EncoderOption {
  id: string
  label: string
  vendor: string
  kind: string
  available: boolean
  reason: string
}

export interface TaskView {
  id: string
  state: string
  inputPath: string
  inputName: string
  inputSize: number
  outputPath: string
  mode: string
  codec: string
  container: string
  quality: string
  encoder: string
  encoderArg: string
  bitrateMbps: number
  percent: number
  speed: number
  fps: number
  outSize: number
  bitrateKbps: number
  etaSec: number
  durationSec: number
  warnings: string[]
  errCode: string
  errMessage: string
  createdAt: number
  startedAt: number
  endedAt: number
  encodeTimeSec: number
  fallbackUsed: boolean
}

export interface HistoryEntry {
  id: number
  createdAt: string
  inputName: string
  inputPath: string
  inputSize: number
  outputName: string
  outputPath: string
  outputSize: number
  container: string
  videoCodec: string
  encoder: string
  quality: string
  bitrateKbps: number
  durationSec: number
  encodeTimeSec: number
  ratio: number
  status: string
  errorSummary: string
  warnings: string
}

export interface Settings {
  language: string
  theme: string
  outputDir: string
  conflictStrategy: string
  rememberConflict: boolean
  defaultCodec: string
  hardwareEncode: boolean
  ffmpegPath: string
  ffprobePath: string
}

export interface AppInfo {
  name: string
  version: string
  ffmpegPath: string
  ffprobePath: string
  ffmpegOk: boolean
  platform: string
}

export interface AuthSnapshot {
  state: string
  accountEmail: string
  canUseSimple: boolean
  canUseProfessional: boolean
  reason: string
}

export interface LogsUsage {
  totalBytes: number
  fileCount: number
}

export interface UpdateCheck {
  currentVersion: string
  latestVersion: string
  upToDate: boolean
  note: string
}
