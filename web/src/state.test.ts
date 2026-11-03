import { describe, expect, it, vi } from 'vitest'
import { Store } from './state'

describe('Store', () => {
  it('calls a new subscriber immediately, then on every set', () => {
    const s = new Store(1)
    const fn = vi.fn()
    s.subscribe(fn)
    s.set(2)
    expect(fn.mock.calls).toEqual([[1], [2]])
    expect(s.get()).toBe(2)
  })

  it('stops notifying after unsubscribe', () => {
    const s = new Store('a')
    const fn = vi.fn()
    const off = s.subscribe(fn)
    off()
    s.set('b')
    expect(fn).toHaveBeenCalledTimes(1)
  })

  it('notifies every subscriber', () => {
    const s = new Store(0)
    const a = vi.fn()
    const b = vi.fn()
    s.subscribe(a)
    s.subscribe(b)
    s.set(5)
    expect(a).toHaveBeenLastCalledWith(5)
    expect(b).toHaveBeenLastCalledWith(5)
  })
})
