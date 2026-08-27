import type { Entry } from './api'

type Listener<T> = (value: T) => void

// Store is a minimal pub-sub cell: components subscribe to be called
// immediately with the current value and again on every change, without
// pulling in a framework for what's a handful of shared values.
export class Store<T> {
  private value: T
  private listeners = new Set<Listener<T>>()

  constructor(initial: T) {
    this.value = initial
  }

  get(): T {
    return this.value
  }

  set(value: T): void {
    this.value = value
    for (const listener of this.listeners) listener(value)
  }

  subscribe(listener: Listener<T>): () => void {
    this.listeners.add(listener)
    listener(this.value)
    return () => this.listeners.delete(listener)
  }
}

// selectedVideos holds the checked-but-not-yet-submitted videos in the
// directory browser (cleared once a batch of jobs is started).
export const selectedVideos = new Store<Entry[]>([])

// TabInfo is one entry in openTabs — carrying videoName alongside the job id
// so progressPanel's tab strip doesn't need an extra fetch just for a label.
export interface TabInfo {
  id: string
  videoName: string
}

// openTabs is every job currently shown as a Progress tab (new jobs from a
// batch submit, or older jobs reopened from History); activeTabId is which
// one is frontmost. Replaces the old single-job activeJobId now that several
// jobs can be in flight/viewable at once.
export const openTabs = new Store<TabInfo[]>([])
export const activeTabId = new Store<string | null>(null)
