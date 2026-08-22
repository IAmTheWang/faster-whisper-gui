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

export const selectedVideo = new Store<Entry | null>(null)
export const activeJobId = new Store<string | null>(null)
