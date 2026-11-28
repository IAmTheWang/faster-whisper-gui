// Job lifecycle states shared by the API client and the SSE stream. Kept in its
// own module so jobStream.ts can validate wire data without importing api.ts
// (which imports jobStream.ts).
export type JobStatus =
  | 'queued'
  | 'extracting_audio'
  | 'transcribing'
  | 'paused'
  | 'done'
  | 'failed'
  | 'canceled'

export const jobStatusLabels: Record<JobStatus, string> = {
  queued: 'Queued',
  extracting_audio: 'Extracting audio',
  transcribing: 'Transcribing',
  paused: 'Paused',
  done: 'Done',
  failed: 'Failed',
  canceled: 'Canceled',
}

export function isJobStatus(value: unknown): value is JobStatus {
  return typeof value === 'string' && Object.hasOwn(jobStatusLabels, value)
}

export function isTerminalStatus(status: JobStatus): boolean {
  return status === 'done' || status === 'failed' || status === 'canceled'
}
