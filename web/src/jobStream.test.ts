import { describe, expect, it, vi } from 'vitest'
import {
  createJobStream,
  isDoneData,
  isErrorData,
  isProgressData,
  parseFrame,
  type EventSourceLike,
  type JobEventHandlers,
} from './jobStream'

// A controllable stand-in for EventSource.
class FakeSource implements EventSourceLike {
  readyState = 0
  closed = false
  private listeners = new Map<string, ((e: Event) => void)[]>()

  addEventListener(type: string, listener: (e: Event) => void): void {
    this.listeners.set(type, [...(this.listeners.get(type) ?? []), listener])
  }
  close(): void {
    this.closed = true
    this.readyState = 2
  }
  open(): void {
    this.readyState = 1
    this.emit('open')
  }
  emit(type: string, data?: unknown): void {
    const e = (data === undefined ? new Event(type) : Object.assign(new Event(type), { data })) as Event
    for (const l of this.listeners.get(type) ?? []) l(e)
  }
  frame(type: string, jobId: string, data: unknown = {}): void {
    this.emit(type, JSON.stringify({ jobId, data }))
  }
}

function setup() {
  const sources: FakeSource[] = []
  const stream = createJobStream(() => {
    const s = new FakeSource()
    sources.push(s)
    return s
  })
  return { stream, sources }
}

const progress = { stage: 'transcribing', percent: 40, message: 'hi' }

describe('createJobStream', () => {
  it('opens one source for any number of jobs and routes events by job id', () => {
    const { stream, sources } = setup()
    const a = { onProgress: vi.fn() }
    const b = { onProgress: vi.fn() }
    stream.subscribe('a', a)
    stream.subscribe('b', b)
    expect(sources).toHaveLength(1)

    sources[0]!.frame('progress', 'a', progress)
    expect(a.onProgress).toHaveBeenCalledWith(progress)
    expect(b.onProgress).not.toHaveBeenCalled()
  })

  it('closes the source when the last subscription goes away, and reopens for the next', () => {
    const { stream, sources } = setup()
    const off1 = stream.subscribe('a', {})
    const off2 = stream.subscribe('b', {})
    off1()
    expect(sources[0]!.closed).toBe(false)
    off2()
    expect(sources[0]!.closed).toBe(true)
    stream.subscribe('c', {})
    expect(sources).toHaveLength(2)
  })

  it('ends a job subscription after its terminal event', () => {
    const { stream, sources } = setup()
    const a: JobEventHandlers = { onDone: vi.fn(), onProgress: vi.fn() }
    stream.subscribe('a', a)
    stream.subscribe('b', {})
    sources[0]!.frame('done', 'a', { srtPath: 'x.srt' })
    expect(a.onDone).toHaveBeenCalledWith({ srtPath: 'x.srt' })
    sources[0]!.frame('progress', 'a', progress)
    expect(a.onProgress).not.toHaveBeenCalled()
    expect(sources[0]!.closed).toBe(false) // b is still listening
  })

  it('closes the connection when the only job finishes', () => {
    const { stream, sources } = setup()
    stream.subscribe('a', { onCanceled: vi.fn() })
    sources[0]!.frame('canceled', 'a')
    expect(sources[0]!.closed).toBe(true)
  })

  it('tells a server "error" event from a native connection error', () => {
    const { stream, sources } = setup()
    const h = { onError: vi.fn(), onDisconnect: vi.fn() }
    stream.subscribe('a', h)

    sources[0]!.readyState = 0 // transient drop: EventSource is reconnecting
    sources[0]!.emit('error')
    expect(h.onError).not.toHaveBeenCalled()
    expect(h.onDisconnect).not.toHaveBeenCalled()

    sources[0]!.frame('error', 'a', { message: 'boom' })
    expect(h.onError).toHaveBeenCalledWith({ message: 'boom' })
  })

  it('reports a permanently closed stream to every subscriber, then starts fresh', () => {
    const { stream, sources } = setup()
    const a = { onDisconnect: vi.fn() }
    const b = { onDisconnect: vi.fn() }
    stream.subscribe('a', a)
    stream.subscribe('b', b)

    sources[0]!.readyState = 2
    sources[0]!.emit('error')
    expect(a.onDisconnect).toHaveBeenCalledTimes(1)
    expect(b.onDisconnect).toHaveBeenCalledTimes(1)
    expect(sources[0]!.closed).toBe(true)

    stream.subscribe('c', {})
    expect(sources).toHaveLength(2)
  })

  it('fires onSync when the stream opens and again after a reconnect', () => {
    const { stream, sources } = setup()
    const h = { onSync: vi.fn() }
    stream.subscribe('a', h)
    expect(h.onSync).not.toHaveBeenCalled()
    sources[0]!.open()
    expect(h.onSync).toHaveBeenCalledTimes(1)
    sources[0]!.open() // EventSource reconnected by itself
    expect(h.onSync).toHaveBeenCalledTimes(2)
  })

  it('syncs a late subscriber of an already-open stream', async () => {
    const { stream, sources } = setup()
    stream.subscribe('a', {})
    sources[0]!.open()
    const late = { onSync: vi.fn() }
    stream.subscribe('b', late)
    await Promise.resolve()
    expect(late.onSync).toHaveBeenCalledTimes(1)
  })

  it('does not sync a subscriber that unsubscribed before the microtask ran', async () => {
    const { stream, sources } = setup()
    stream.subscribe('a', {})
    sources[0]!.open()
    const late = { onSync: vi.fn() }
    stream.subscribe('b', late)()
    await Promise.resolve()
    expect(late.onSync).not.toHaveBeenCalled()
  })

  it('ignores malformed frames and payloads instead of throwing', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const { stream, sources } = setup()
    const h = { onProgress: vi.fn(), onDone: vi.fn() }
    stream.subscribe('a', h)

    sources[0]!.emit('progress', 'not json')
    sources[0]!.emit('progress', JSON.stringify({ nojob: 1 }))
    sources[0]!.frame('progress', 'a', { stage: 'bogus', percent: 'x' })
    sources[0]!.frame('done', 'a', {})
    expect(h.onProgress).not.toHaveBeenCalled()
    expect(h.onDone).not.toHaveBeenCalled()
    expect(warn).toHaveBeenCalled()
    warn.mockRestore()
  })

  it('keeps notifying other handlers when one throws', () => {
    const err = vi.spyOn(console, 'error').mockImplementation(() => {})
    const { stream, sources } = setup()
    const good = { onProgress: vi.fn() }
    stream.subscribe('a', {
      onProgress: () => {
        throw new Error('bad tab')
      },
    })
    stream.subscribe('a', good)
    sources[0]!.frame('progress', 'a', progress)
    expect(good.onProgress).toHaveBeenCalled()
    err.mockRestore()
  })

  it('ignores events from a source that has been replaced', () => {
    const { stream, sources } = setup()
    const off = stream.subscribe('a', {})
    off() // closes sources[0]
    const h = { onProgress: vi.fn() }
    stream.subscribe('a', h)
    sources[0]!.frame('progress', 'a', progress) // stale source
    expect(h.onProgress).not.toHaveBeenCalled()
  })
})

describe('payload guards', () => {
  it('accepts well-formed payloads only', () => {
    expect(isProgressData(progress)).toBe(true)
    expect(isProgressData({ ...progress, stage: 'nope' })).toBe(false)
    expect(isProgressData(null)).toBe(false)
    expect(isDoneData({ srtPath: 'a.srt' })).toBe(true)
    expect(isDoneData({})).toBe(false)
    expect(isErrorData({ message: 'x' })).toBe(true)
    expect(isErrorData({ message: 1 })).toBe(false)
  })

  it('parseFrame requires JSON text with a job id', () => {
    expect(parseFrame(JSON.stringify({ jobId: 'j', data: 1 }))).toEqual({ jobId: 'j', data: 1 })
    expect(parseFrame('nope')).toBeNull()
    expect(parseFrame(JSON.stringify({ data: 1 }))).toBeNull()
    expect(parseFrame(undefined)).toBeNull()
  })
})
