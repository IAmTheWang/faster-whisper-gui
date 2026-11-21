import type { Job } from '../../api'
import type { DoneEventData, ErrorEventData, ProgressEventData } from '../../jobStream'
import { isTerminalStatus, type JobStatus } from '../../jobStatus'

// The state of one Progress tab, and the pure transitions on it. Kept free of
// DOM and network so every transition (including the awkward ones — a stale
// snapshot, a lost connection) can be unit-tested.

export interface SaveStatus {
  text: string
  kind: 'success' | 'error'
}

export interface TabState {
  status: JobStatus
  percent: number
  message: string
  srtPath?: string
  error?: string
  // Kept per tab (not in the DOM) so the panel can redraw on every progress
  // event or tab switch without losing them.
  saveStatus?: SaveStatus
  cancelError?: string
  // True once a snapshot from the server or a live event has been applied; a
  // failed first load (never synced) is shown as an error on the tab.
  synced: boolean
  // Live events applied so far. A snapshot fetched before the latest event
  // would be older than the state it overwrites, so the caller compares this
  // counter before and after the fetch and drops the stale snapshot.
  events: number
  unsubscribe: () => void
}

export function newTabState(): TabState {
  return { status: 'queued', percent: 0, message: '', synced: false, events: 0, unsubscribe: () => {} }
}

export function applySnapshot(state: TabState, job: Pick<Job, 'status' | 'percent' | 'message' | 'srtPath' | 'error'>): void {
  state.status = job.status
  state.percent = job.percent
  state.message = job.message ?? ''
  state.srtPath = job.srtPath
  state.error = job.error
  state.synced = true
}

export function applyProgress(state: TabState, data: ProgressEventData): void {
  state.events++
  state.synced = true
  state.status = data.stage
  state.percent = data.percent
  state.message = data.message
}

export function applyDone(state: TabState, data: DoneEventData): void {
  state.events++
  state.synced = true
  state.status = 'done'
  state.percent = 100
  state.srtPath = data.srtPath
}

export function applyError(state: TabState, data: ErrorEventData): void {
  state.events++
  state.synced = true
  state.status = 'failed'
  state.error = data.message
}

export function applyCanceled(state: TabState): void {
  state.events++
  state.synced = true
  state.status = 'canceled'
}

// applyFailure marks the tab failed for a reason that comes from this client
// rather than from the job (it could not be loaded, the connection was lost).
export function applyFailure(state: TabState, reason: string): void {
  state.status = 'failed'
  state.error = reason
}

export const LOST_CONNECTION = 'Lost connection to the server while the job was running.'
export const UNKNOWN_JOB = 'The server no longer knows this job (it may have been restarted).'

// isNotFound recognizes the server's "job not found" answer from the message of
// the Error api.ts throws (the HTTP status is not carried on it).
export function isNotFound(err: unknown): boolean {
  return err instanceof Error && /not found/i.test(err.message)
}

export function isFinished(state: Pick<TabState, 'status'>): boolean {
  return isTerminalStatus(state.status)
}
