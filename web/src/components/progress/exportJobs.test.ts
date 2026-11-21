import { describe, expect, it, vi } from 'vitest'
import { exportCompleted, summarizeExport } from './exportJobs'

const tabs = [
  { id: 'a', mediaName: 'a.mp4' },
  { id: 'b', mediaName: 'b.mp4' },
  { id: 'c', mediaName: 'c.mp4' },
]

function states(map: Record<string, { status: 'done' | 'failed' | 'transcribing'; srtPath?: string }>) {
  return (id: string) => map[id]
}

describe('exportCompleted', () => {
  it('exports only completed tabs', async () => {
    const exportJob = vi.fn().mockResolvedValue({})
    const out = await exportCompleted(
      exportJob,
      tabs,
      states({
        a: { status: 'done', srtPath: 'E:\\v\\a.srt' },
        b: { status: 'transcribing' },
        c: { status: 'failed' },
      }),
      'D:\\out',
    )
    expect(exportJob).toHaveBeenCalledTimes(1)
    expect(exportJob).toHaveBeenCalledWith('a', 'D:\\out')
    expect(out).toEqual({ succeeded: 1, total: 1, problems: [] })
  })

  it('skips a later job whose .srt would overwrite an earlier copy (case-insensitive)', async () => {
    const exportJob = vi.fn().mockResolvedValue({})
    const out = await exportCompleted(
      exportJob,
      tabs,
      states({
        a: { status: 'done', srtPath: 'E:\\one\\Clip.srt' },
        b: { status: 'done', srtPath: 'E:\\two\\clip.SRT' },
        c: { status: 'done', srtPath: 'E:\\two\\other.srt' },
      }),
      'D:\\out',
    )
    expect(exportJob.mock.calls.map((c) => c[0])).toEqual(['a', 'c'])
    expect(out.succeeded).toBe(2)
    expect(out.total).toBe(3)
    expect(out.problems).toHaveLength(1)
    expect(out.problems[0]).toContain('b.mp4: skipped')
    expect(out.problems[0]).toContain('a.mp4')
  })

  it('a failed export does not claim its name for later jobs', async () => {
    const exportJob = vi.fn().mockRejectedValueOnce(new Error('disk full')).mockResolvedValue({})
    const out = await exportCompleted(
      exportJob,
      tabs.slice(0, 2),
      states({
        a: { status: 'done', srtPath: 'E:\\one\\clip.srt' },
        b: { status: 'done', srtPath: 'E:\\two\\clip.srt' },
      }),
      'D:\\out',
    )
    expect(exportJob).toHaveBeenCalledTimes(2)
    expect(out).toEqual({ succeeded: 1, total: 2, problems: ['a.mp4: disk full'] })
  })
})

describe('summarizeExport', () => {
  it('reports success or lists the problems', () => {
    expect(summarizeExport({ succeeded: 2, total: 2, problems: [] })).toEqual({ text: 'Saved 2/2', kind: 'success' })
    expect(summarizeExport({ succeeded: 1, total: 2, problems: ['x', 'y'] })).toEqual({
      text: 'Saved 1/2 — x; y',
      kind: 'error',
    })
  })
})
