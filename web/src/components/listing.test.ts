import { describe, expect, it } from 'vitest'
import { driveFor, entryIcon, Sequencer } from './listing'

describe('Sequencer', () => {
  it('only the newest ticket is current', () => {
    const seq = new Sequencer()
    const first = seq.next()
    const second = seq.next()
    expect(seq.isCurrent(first)).toBe(false)
    expect(seq.isCurrent(second)).toBe(true)
  })

  it('invalidate drops every ticket still in flight', () => {
    const seq = new Sequencer()
    const t = seq.next()
    seq.invalidate()
    expect(seq.isCurrent(t)).toBe(false)
  })
})

describe('entryIcon', () => {
  it('has an icon per entry type', () => {
    expect(entryIcon('dir')).toBe('📁')
    expect(entryIcon('video')).toBe('🎬')
    expect(entryIcon('audio')).toBe('🎵')
    expect(entryIcon('file')).toBe('📄')
  })
})

describe('driveFor', () => {
  const drives = [
    { name: 'C:\\', label: '' },
    { name: 'E:\\', label: 'Data' },
  ]
  it('matches the drive a path lives on, ignoring case', () => {
    expect(driveFor(drives, 'e:\\videos')?.name).toBe('E:\\')
    expect(driveFor(drives, 'C:\\')?.name).toBe('C:\\')
  })
  it('returns undefined for an unknown drive', () => {
    expect(driveFor(drives, 'Z:\\x')).toBeUndefined()
  })
})
