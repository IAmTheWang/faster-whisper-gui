export interface Drive {
  name: string
  label: string
}

export interface Entry {
  name: string
  path: string
  type: 'dir' | 'video' | 'audio' | 'file'
  size: number
  modTime: number
  createdTime: number
}

export interface Listing {
  path: string
  parent?: string
  entries: Entry[]
}

export type BrowseKind = 'media' | 'exe' | 'dir'

export interface SettingsPaths {
  ffmpegPath: string
  whisperCliPath: string
  modelsDir: string
  defaultVideoDir: string
  ffmpegDefault: string
  whisperCliDefault: string
  modelsDirDefault: string
}

export interface UpdateSettingsRequest {
  ffmpegPath?: string
  whisperCliPath?: string
  modelsDir?: string
  defaultVideoDir?: string
}

export interface Model {
  id: string
  filename: string
  path: string
  sizeBytes: number
  label: string
}

export interface Language {
  code: string
  label: string
}

export interface ComponentHealth {
  ok: boolean
  detail: string
}

export interface HealthResponse {
  ffmpeg: ComponentHealth
  whisperCli: ComponentHealth
}

export type OutputMode = 'same_as_source' | 'custom'

export type JobStatus =
  | 'queued'
  | 'extracting_audio'
  | 'transcribing'
  | 'done'
  | 'failed'
  | 'canceled'

export interface JobRequestView {
  mediaPath: string
  modelId: string
  language: string
  outputMode: OutputMode
  outputDir?: string
  maxLen?: number
}

export interface Job {
  id: string
  request: JobRequestView
  status: JobStatus
  percent: number
  message?: string
  srtPath?: string
  error?: string
  createdAt: string
  updatedAt: string
}

export interface CreateJobRequest {
  mediaPath: string
  modelId: string
  language: string
  outputMode: OutputMode
  outputDir?: string
  maxLen?: number
}

export interface CreateJobResponse {
  jobId: string
  status: JobStatus
}

async function errorMessageFrom(res: Response): Promise<string> {
  try {
    const body = await res.json()
    if (body && typeof body.error === 'string') return body.error
  } catch {
    // response body wasn't JSON — fall back below
  }
  return res.statusText || `HTTP ${res.status}`
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, init)
  if (!res.ok) {
    throw new Error(await errorMessageFrom(res))
  }
  return res.json() as Promise<T>
}

export const api = {
  health: () => request<HealthResponse>('/api/health'),
  drives: () => request<Drive[]>('/api/drives'),
  browse: (path: string, kind: BrowseKind = 'media') =>
    request<Listing>(`/api/browse?path=${encodeURIComponent(path)}&kind=${kind}`),
  models: () => request<Model[]>('/api/models'),
  languages: () => request<Language[]>('/api/languages'),
  jobs: () => request<Job[]>('/api/jobs'),
  job: (id: string) => request<Job>(`/api/jobs/${encodeURIComponent(id)}`),
  settings: () => request<SettingsPaths>('/api/settings'),

  updateSettings(body: UpdateSettingsRequest): Promise<SettingsPaths> {
    return request<SettingsPaths>('/api/settings', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
  },

  createJob(body: CreateJobRequest): Promise<CreateJobResponse> {
    return request<CreateJobResponse>('/api/jobs', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
  },

  async cancelJob(id: string): Promise<void> {
    const res = await fetch(`/api/jobs/${encodeURIComponent(id)}/cancel`, { method: 'POST' })
    if (!res.ok) {
      throw new Error(await errorMessageFrom(res))
    }
  },

  exportJob(id: string, destDir: string): Promise<ExportJobResponse> {
    return request<ExportJobResponse>(`/api/jobs/${encodeURIComponent(id)}/export`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ destDir }),
    })
  },
}

export interface ExportJobResponse {
  destPath: string
}

export interface ProgressEventData {
  stage: JobStatus
  percent: number
  message: string
}

export interface DoneEventData {
  srtPath: string
}

export interface ErrorEventData {
  message: string
}

export interface JobEventHandlers {
  onProgress?: (data: ProgressEventData) => void
  onDone?: (data: DoneEventData) => void
  onError?: (data: ErrorEventData) => void
  onCanceled?: () => void
}

// subscribeJobEvents wraps EventSource for one job's SSE stream. The
// server's custom "error" event unavoidably shares a name with
// EventSource's built-in connection-failure event; a genuine SSE-carried
// error always arrives as a MessageEvent with a string `data` payload,
// while a native transport error does not, so that's what distinguishes
// them below rather than trying to rename the wire event.
export function subscribeJobEvents(jobId: string, handlers: JobEventHandlers): () => void {
  const source = new EventSource(`/api/jobs/${encodeURIComponent(jobId)}/events`)

  source.addEventListener('progress', (e) => {
    handlers.onProgress?.(JSON.parse((e as MessageEvent).data))
  })
  source.addEventListener('done', (e) => {
    handlers.onDone?.(JSON.parse((e as MessageEvent).data))
    source.close()
  })
  source.addEventListener('error', (e) => {
    const data = (e as MessageEvent).data
    if (typeof data === 'string') {
      handlers.onError?.(JSON.parse(data))
      source.close()
    }
  })
  source.addEventListener('canceled', () => {
    handlers.onCanceled?.()
    source.close()
  })

  return () => source.close()
}
