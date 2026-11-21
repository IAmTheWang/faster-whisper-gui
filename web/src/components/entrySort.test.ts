import { describe, expect, it } from 'vitest'
import type { Entry } from '../api'
import {
  compareEntries,
  defaultDirFor,
  isSortKey,
  loadStoredSort,
  SORT_STORAGE_KEY,
  storeSort,
} from './entrySort'

const e = (name: string, type: Entry['type'], over: Partial<Entry> = {}): Entry => ({
  name,
  path: `C:\\${name}`,
  type,
  size: 0,
  modTime: 0,
  createdTime: 0,
  ...over,
})

const names = (entries: Entry[]) => entries.map((x) => x.name)

describe('compareEntries', () => {
  const list = [
    e('b.mp4', 'video', { size: 5, modTime: 20 }),
    e('Zeta', 'dir'),
    e('a.mp3', 'audio', { size: 9, modTime: 10 }),
    e('alpha', 'dir'),
  ]

  it('always puts directories first, then sorts by name case-insensitively', () => {
    expect(names([...list].sort(compareEntries('name', 'asc')))).toEqual(['alpha', 'Zeta', 'a.mp3', 'b.mp4'])
  })

  it('reverses the order of files but keeps directories first when descending', () => {
    const out = names([...list].sort(compareEntries('size', 'desc')))
    expect(out.slice(2)).toEqual(['a.mp3', 'b.mp4'])
    expect(out.slice(0, 2).sort()).toEqual(['Zeta', 'alpha'])
  })

  it('sorts by modification time and breaks ties by name', () => {
    const tied = [e('b', 'video', { modTime: 1 }), e('a', 'video', { modTime: 1 }), e('c', 'video', { modTime: 0 })]
    expect(names(tied.sort(compareEntries('modTime', 'asc')))).toEqual(['c', 'a', 'b'])
  })

  it('sorts by type', () => {
    expect(names([e('x', 'video'), e('y', 'audio')].sort(compareEntries('type', 'asc')))).toEqual(['y', 'x'])
  })
})

describe('sort key helpers', () => {
  it('validates keys and picks newest-first for dates', () => {
    expect(isSortKey('size')).toBe(true)
    expect(isSortKey('nope')).toBe(false)
    expect(isSortKey(undefined)).toBe(false)
    expect(defaultDirFor('modTime')).toBe('desc')
    expect(defaultDirFor('createdTime')).toBe('desc')
    expect(defaultDirFor('name')).toBe('asc')
  })
})

describe('stored sort', () => {
  const memory = (initial?: string) => {
    const data = new Map<string, string>()
    if (initial !== undefined) data.set(SORT_STORAGE_KEY, initial)
    return {
      getItem: (k: string) => data.get(k) ?? null,
      setItem: (k: string, v: string) => void data.set(k, v),
    }
  }

  it('round-trips', () => {
    const s = memory()
    storeSort(s, 'size', 'desc')
    expect(loadStoredSort(s)).toEqual({ key: 'size', dir: 'desc' })
  })

  it('rejects missing, corrupt and out-of-range values', () => {
    expect(loadStoredSort(memory())).toBeNull()
    expect(loadStoredSort(memory('not json'))).toBeNull()
    expect(loadStoredSort(memory('null'))).toBeNull()
    expect(loadStoredSort(memory(JSON.stringify({ key: 'bogus', dir: 'asc' })))).toBeNull()
    expect(loadStoredSort(memory(JSON.stringify({ key: 'name', dir: 'sideways' })))).toBeNull()
  })

  it('survives a storage that throws', () => {
    const broken = {
      getItem: () => {
        throw new Error('denied')
      },
      setItem: () => {
        throw new Error('quota')
      },
    }
    expect(loadStoredSort(broken)).toBeNull()
    expect(() => storeSort(broken, 'name', 'asc')).not.toThrow()
  })
})
