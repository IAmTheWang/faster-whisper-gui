import { isJobStatus, type JobStatus } from './jobStatus'

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
  // The stream is (re)connected and live: the moment to fetch the job's current
  // state, since events published while it was down are gone. Fired when the
  // stream first opens, after every automatic reconnect, and right after
  // subscribing to an already-open stream.
  onSync?: () => void
  // The stream closed for good (e.g. the server rejected it, or restarted with
  // a new API token). All subscriptions are dropped; resubscribe to retry.
  onDisconnect?: () => void
}

// The slice of EventSource this module uses, so tests can inject a fake.
export interface EventSourceLike {
  readonly readyState: number
  addEventListener(type: string, listener: (e: Event) => void): void
  close(): void
}

const ES_OPEN = 1
const ES_CLOSED = 2

export interface JobStream {
  subscribe(jobId: string, handlers: JobEventHandlers): () => void
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

export function isProgressData(d: unknown): d is ProgressEventData {
  return isRecord(d) && isJobStatus(d.stage) && typeof d.percent === 'number' && typeof d.message === 'string'
}

export function isDoneData(d: unknown): d is DoneEventData {
  return isRecord(d) && typeof d.srtPath === 'string'
}

export function isErrorData(d: unknown): d is ErrorEventData {
  return isRecord(d) && typeof d.message === 'string'
}

// One server frame: the per-job payload plus the job it belongs to.
export function parseFrame(raw: unknown): { jobId: string; data: unknown } | null {
  if (typeof raw !== 'string') return null
  try {
    const frame: unknown = JSON.parse(raw)
    if (isRecord(frame) && typeof frame.jobId === 'string') return { jobId: frame.jobId, data: frame.data }
  } catch {
    // fall through: not JSON
  }
  return null
}

function safely(fn: (() => void) | undefined): void {
  if (!fn) return
  try {
    fn()
  } catch (err) {
    // One misbehaving tab must not stop the others from being notified.
    console.error('job event handler failed', err)
  }
}

// createJobStream multiplexes every job's events over ONE EventSource (the
// server's GET /api/events): browsers cap concurrent HTTP/1.1 connections per
// host at about six, and a connection per open Progress tab would starve the
// ordinary fetch() calls — including Cancel.
//
// The source is opened on the first subscription and closed when the last one
// goes away. A job's subscription ends by itself after its terminal event.
//
// The server's custom "error" event unavoidably shares a name with
// EventSource's built-in connection-failure event; a genuine server-sent
// error always arrives as a MessageEvent with a string `data` payload, a
// native transport error does not, which is how they are told apart below.
export function createJobStream(openSource: () => EventSourceLike): JobStream {
  const subs = new Map<string, Set<JobEventHandlers>>()
  let source: EventSourceLike | null = null

  function closeSource(): void {
    source?.close()
    source = null
  }

  function connect(): EventSourceLike {
    const src = openSource()
    source = src

    src.addEventListener('open', () => {
      if (source !== src) return
      for (const set of [...subs.values()]) for (const h of [...set]) safely(h.onSync)
    })

    const onFrame = (type: 'progress' | 'done' | 'error' | 'canceled') => (e: Event) => {
      if (source !== src) return
      const frame = parseFrame((e as MessageEvent).data)
      if (!frame) {
        console.warn(`ignoring malformed ${type} event`)
        return
      }
      deliver(type, frame.jobId, frame.data)
    }
    src.addEventListener('progress', onFrame('progress'))
    src.addEventListener('done', onFrame('done'))
    src.addEventListener('canceled', onFrame('canceled'))

    const onServerError = onFrame('error')
    src.addEventListener('error', (e) => {
      if (source !== src) return
      if (typeof (e as MessageEvent).data === 'string') {
        onServerError(e)
      } else if (src.readyState === ES_CLOSED) {
        const all = [...subs.values()].flatMap((set) => [...set])
        subs.clear()
        closeSource()
        for (const h of all) safely(h.onDisconnect)
      }
      // else: a transient drop; EventSource reconnects by itself and 'open'
      // then triggers onSync, which recovers whatever was missed.
    })
    return src
  }

  function deliver(type: string, jobId: string, data: unknown): void {
    const set = subs.get(jobId)
    if (!set) return
    const handlers = [...set]
    const terminal = type !== 'progress'
    if (terminal) {
      // The job is over: no more events will follow, so drop its subscription
      // (and the connection itself once nothing else is listening).
      subs.delete(jobId)
      if (subs.size === 0) closeSource()
    }
    for (const h of handlers) {
      switch (type) {
        case 'progress':
          if (isProgressData(data)) safely(() => h.onProgress?.(data))
          else console.warn('ignoring malformed progress payload')
          break
        case 'done':
          if (isDoneData(data)) safely(() => h.onDone?.(data))
          else console.warn('ignoring malformed done payload')
          break
        case 'error':
          if (isErrorData(data)) safely(() => h.onError?.(data))
          else console.warn('ignoring malformed error payload')
          break
        case 'canceled':
          safely(h.onCanceled)
          break
      }
    }
  }

  return {
    subscribe(jobId, handlers) {
      let set = subs.get(jobId)
      if (!set) subs.set(jobId, (set = new Set()))
      set.add(handlers)

      if (!source) {
        connect() // onSync fires from its 'open' event
      } else if (source.readyState === ES_OPEN) {
        const src = source
        queueMicrotask(() => {
          if (source === src && subs.get(jobId)?.has(handlers)) safely(handlers.onSync)
        })
      }

      return () => {
        const current = subs.get(jobId)
        if (!current) return
        current.delete(handlers)
        if (current.size === 0) subs.delete(jobId)
        if (subs.size === 0) closeSource()
      }
    },
  }
}
