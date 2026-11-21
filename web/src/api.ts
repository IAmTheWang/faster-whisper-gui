import { createJobStream, type JobEventHandlers } from './jobStream'
import type { JobStatus } from './jobStatus'

export { isTerminalStatus, jobStatusLabels, type JobStatus } from './jobStatus'
export type {
  DoneEventData,
  ErrorEventData,
  JobEventHandlers,
  ProgressEventData,
} from './jobStream'

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

export interface JobRequestView {
  mediaPath: string
  modelId: string
  language: string
  outputMode: OutputMode
  outputDir?: string
  maxLen?: number
  maxContext?: number
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

// A job request is sent and echoed back field-for-field identical.
export type CreateJobRequest = JobRequestView

export interface CreateJobResponse {
  jobId: string
  status: JobStatus
}

export interface ExportJobResponse {
  destPath: string
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

// The Go server swaps __API_TOKEN__ in index.html's <meta name="api-token"> for
// a per-process secret (Vite's dev server does the same via vite.config.ts);
// every /api call must carry it. A page that never saw index.html — i.e. any
// other website — has no way to learn it. Read lazily so importing this module
// needs no DOM.
let cachedToken: string | undefined
function apiToken(): string {
  if (cachedToken === undefined) {
    const v =
      typeof document === 'undefined'
        ? ''
        : (document.querySelector('meta[name="api-token"]')?.getAttribute('content') ?? '')
    cachedToken = v === '__API_TOKEN__' ? '' : v
  }
  return cachedToken
}

function withToken(init?: RequestInit): RequestInit {
  const headers = new Headers(init?.headers)
  const token = apiToken()
  if (token) headers.set('X-Api-Token', token)
  return { ...init, headers }
}

// A request that never answers would leave the UI waiting forever (the buttons
// that fired it stay disabled), so every call has a deadline.
const REQUEST_TIMEOUT_MS = 30_000

async function apiFetch(path: string, init?: RequestInit): Promise<Response> {
  const controller = new AbortController()
  const timer = setTimeout(() => controller.abort(), REQUEST_TIMEOUT_MS)
  try {
    return await fetch(path, { ...withToken(init), signal: controller.signal })
  } catch (err) {
    if (controller.signal.aborted) {
      throw new Error(`The server did not answer within ${REQUEST_TIMEOUT_MS / 1000}s`)
    }
    throw err
  } finally {
    clearTimeout(timer)
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await apiFetch(path, init)
  if (!res.ok) {
    throw new Error(await errorMessageFrom(res))
  }
  try {
    return (await res.json()) as T
  } catch {
    throw new Error('The server sent an unreadable response')
  }
}

function jsonRequest(method: string, body: unknown): RequestInit {
  return { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }
}

export const api = {
  health: () => request<HealthResponse>('/api/health'),
  drives: () => request<Drive[] | null>('/api/drives').then((d) => d ?? []),
  browse: (path: string, kind: BrowseKind = 'media') =>
    request<Listing>(`/api/browse?path=${encodeURIComponent(path)}&kind=${kind}`),
  models: () => request<Model[]>('/api/models'),
  languages: () => request<Language[]>('/api/languages'),
  jobs: () => request<Job[]>('/api/jobs'),
  job: (id: string) => request<Job>(`/api/jobs/${encodeURIComponent(id)}`),
  settings: () => request<SettingsPaths>('/api/settings'),

  updateSettings(body: UpdateSettingsRequest): Promise<SettingsPaths> {
    return request<SettingsPaths>('/api/settings', jsonRequest('PUT', body))
  },

  createJob(body: CreateJobRequest): Promise<CreateJobResponse> {
    return request<CreateJobResponse>('/api/jobs', jsonRequest('POST', body))
  },

  async cancelJob(id: string): Promise<void> {
    const res = await apiFetch(`/api/jobs/${encodeURIComponent(id)}/cancel`, { method: 'POST' })
    if (!res.ok) {
      throw new Error(await errorMessageFrom(res))
    }
  },

  exportJob(id: string, destDir: string): Promise<ExportJobResponse> {
    return request<ExportJobResponse>(`/api/jobs/${encodeURIComponent(id)}/export`, jsonRequest('POST', { destDir }))
  },
}

// One shared EventSource for every job (see jobStream.ts). EventSource can't
// set headers, so the token travels in the query string; the server accepts
// that only for GET .../events and never logs query strings.
const jobStream = createJobStream(() => {
  const token = apiToken()
  return new EventSource(token ? `/api/events?token=${encodeURIComponent(token)}` : '/api/events')
})

// subscribeJobEvents delivers one job's events; call the returned function to
// stop. A job's subscription also ends by itself after its terminal event.
export function subscribeJobEvents(jobId: string, handlers: JobEventHandlers): () => void {
  return jobStream.subscribe(jobId, handlers)
}
