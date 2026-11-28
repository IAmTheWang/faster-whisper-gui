import { describe, expect, it } from 'vitest'
import { summarizeFailures } from './jobFailures'

describe('summarizeFailures', () => {
  it('returns nothing for no failures', () => {
    expect(summarizeFailures([])).toBe('')
  })

  it('keeps a lone failure as "name: message"', () => {
    expect(summarizeFailures([{ name: 'a.mp4', message: 'locked' }])).toBe('a.mp4: locked')
  })

  it('keeps distinct messages separate, in first-seen order', () => {
    expect(
      summarizeFailures([
        { name: 'a.mp4', message: 'locked' },
        { name: 'b.mp4', message: 'missing' },
      ]),
    ).toBe('a.mp4: locked; b.mp4: missing')
  })

  it('groups identical messages and abbreviates long name lists', () => {
    const many = Array.from({ length: 80 }, (_, i) => ({ name: `f${i}.mp4`, message: 'whisper-cli.exe was not found' }))
    expect(summarizeFailures(many)).toBe('80 files: whisper-cli.exe was not found (f0.mp4, f1.mp4, f2.mp4, …)')
    expect(summarizeFailures(many.slice(0, 2))).toBe('2 files: whisper-cli.exe was not found (f0.mp4, f1.mp4)')
  })
})
