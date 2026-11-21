import type { Entry } from '../api'

// Sorting for the media picker's listing, kept free of DOM so it can be tested.

export type SortKey = 'name' | 'type' | 'modTime' | 'createdTime' | 'size'
export type SortDir = 'asc' | 'desc'

export const SORT_KEYS: readonly SortKey[] = ['name', 'type', 'modTime', 'createdTime', 'size']

export const SORT_LABELS: Record<SortKey, string> = {
  name: 'Name',
  type: 'Type',
  modTime: 'Modified',
  createdTime: 'Created',
  size: 'Size',
}

export function isSortKey(value: unknown): value is SortKey {
  return typeof value === 'string' && (SORT_KEYS as readonly string[]).includes(value)
}

// Newest-first is the useful default for dates; everything else starts ascending.
export function defaultDirFor(key: SortKey): SortDir {
  return key === 'modTime' || key === 'createdTime' ? 'desc' : 'asc'
}

function compareByName(a: Entry, b: Entry): number {
  const nameA = a.name.toLowerCase()
  const nameB = b.name.toLowerCase()
  return nameA < nameB ? -1 : nameA > nameB ? 1 : 0
}

// compareEntries returns a comparator: directories always come first, then the
// chosen key (ties broken by name), reversed when dir is 'desc'.
export function compareEntries(key: SortKey, dir: SortDir): (a: Entry, b: Entry) => number {
  return (a, b) => {
    if (a.type === 'dir' && b.type !== 'dir') return -1
    if (a.type !== 'dir' && b.type === 'dir') return 1

    let cmp: number
    switch (key) {
      case 'type':
        cmp = a.type < b.type ? -1 : a.type > b.type ? 1 : compareByName(a, b)
        break
      case 'modTime':
        cmp = a.modTime - b.modTime || compareByName(a, b)
        break
      case 'createdTime':
        cmp = a.createdTime - b.createdTime || compareByName(a, b)
        break
      case 'size':
        cmp = a.size - b.size || compareByName(a, b)
        break
      default:
        cmp = compareByName(a, b)
    }
    return dir === 'asc' ? cmp : -cmp
  }
}

export const SORT_STORAGE_KEY = 'directoryBrowser.sort'

type StorageLike = Pick<Storage, 'getItem' | 'setItem'>

export function loadStoredSort(storage: StorageLike): { key: SortKey; dir: SortDir } | null {
  try {
    const raw = storage.getItem(SORT_STORAGE_KEY)
    if (!raw) return null
    const parsed: unknown = JSON.parse(raw)
    if (typeof parsed !== 'object' || parsed === null) return null
    const { key, dir } = parsed as { key?: unknown; dir?: unknown }
    if (!isSortKey(key) || (dir !== 'asc' && dir !== 'desc')) return null
    return { key, dir }
  } catch {
    return null
  }
}

export function storeSort(storage: StorageLike, key: SortKey, dir: SortDir): void {
  try {
    storage.setItem(SORT_STORAGE_KEY, JSON.stringify({ key, dir }))
  } catch {
    // ignore (e.g. private browsing quota)
  }
}
