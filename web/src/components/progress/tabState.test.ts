import { describe, expect, it } from 'vitest'
import {
  applyCanceled,
  applyDone,
  applyError,
  applyFailure,
  applyProgress,
  applySnapshot,
  isFinished,
  isNotFound,
  newTabState,
} from './tabState'

describe('tab state transitions', () => {
  it('starts queued and unsynced', () => {
    const s = newTabState()
    expect(s).toMatchObject({ status: 'queued', percent: 0, synced: false, events: 0 })
  })

  it('a snapshot syncs the tab without counting as a live event', () => {
    const s = newTabState()
    applySnapshot(s, { status: 'transcribing', percent: 30, message: undefined, srtPath: undefined, error: undefined })
    expect(s).toMatchObject({ status: 'transcribing', percent: 30, message: '', synced: true, events: 0 })
  })

  it('live events bump the counter that guards against stale snapshots', () => {
    const s = newTabState()
    applyProgress(s, { stage: 'transcribing', percent: 10, message: 'a' })
    applyProgress(s, { stage: 'transcribing', percent: 20, message: 'b' })
    expect(s.events).toBe(2)
    expect(s).toMatchObject({ percent: 20, message: 'b', synced: true })
  })

  it('done, error and canceled end the job', () => {
    const done = newTabState()
    applyDone(done, { srtPath: 'x.srt' })
    expect(done).toMatchObject({ status: 'done', percent: 100, srtPath: 'x.srt' })

    const failed = newTabState()
    applyError(failed, { message: 'boom' })
    expect(failed).toMatchObject({ status: 'failed', error: 'boom' })

    const canceled = newTabState()
    applyCanceled(canceled)
    expect(canceled.status).toBe('canceled')

    expect([done, failed, canceled].every(isFinished)).toBe(true)
    expect(isFinished(newTabState())).toBe(false)
  })

  it('applyFailure marks a client-side failure without counting an event', () => {
    const s = newTabState()
    applyFailure(s, 'Could not load job')
    expect(s).toMatchObject({ status: 'failed', error: 'Could not load job', events: 0 })
  })
})

describe('isNotFound', () => {
  it("recognizes the server's job-not-found message only", () => {
    expect(isNotFound(new Error('job not found'))).toBe(true)
    expect(isNotFound(new Error('Failed to fetch'))).toBe(false)
    expect(isNotFound('job not found')).toBe(false)
  })
})
