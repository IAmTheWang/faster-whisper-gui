import { describe, expect, it } from 'vitest'
import type { Job } from '../api'
import { historySignature } from './jobHistory'

const job = (over: Partial<Job> = {}): Job => ({
  id: 'j1',
  request: { mediaPath: 'E:\\v\\a.mp4', modelId: 'tiny', language: 'en', outputMode: 'same_as_source' },
  status: 'transcribing',
  percent: 10.2,
  createdAt: '',
  updatedAt: '',
  ...over,
})

describe('historySignature', () => {
  it('is stable across refreshes that changed nothing visible', () => {
    // Sub-percent differences are not shown (the row rounds), so no rebuild.
    expect(historySignature([job({ percent: 10.2 })], null)).toBe(historySignature([job({ percent: 10.4 })], null))
  })

  it('changes when a row would render differently', () => {
    const base = historySignature([job()], null)
    expect(historySignature([job({ percent: 50 })], null)).not.toBe(base)
    expect(historySignature([job({ status: 'done' })], null)).not.toBe(base)
    expect(historySignature([job()], 'j1')).not.toBe(base) // active highlight
    expect(historySignature([job(), job({ id: 'j2' })], null)).not.toBe(base)
  })

  it('ignores the percent of finished jobs', () => {
    expect(historySignature([job({ status: 'done', percent: 99 })], null)).toBe(
      historySignature([job({ status: 'done', percent: 100 })], null),
    )
  })
})
