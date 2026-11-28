// @vitest-environment happy-dom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Entry } from '../api'

const entry = (name: string, type: Entry['type']): Entry => ({
  name,
  path: `F:\\v\\${name}`,
  type,
  size: 1,
  modTime: 1,
  createdTime: 1,
})

const listing = {
  path: 'F:\\v',
  parent: 'F:\\',
  entries: [entry('sub', 'dir'), entry('a.mp4', 'video'), entry('b.mp4', 'video'), entry('c.mp3', 'audio')],
}

const mocks = vi.hoisted(() => ({ browseError: null as Error | null, browseCalls: [] as string[] }))

vi.mock('../api', () => ({
  api: {
    drives: async () => [{ name: 'F:\\', label: 'F' }],
    settings: async () => ({}),
    browse: async (path: string) => {
      mocks.browseCalls.push(path)
      if (mocks.browseError) throw mocks.browseError
      return listing
    },
  },
}))

// Imported after the mock is registered.
const { mountDirectoryBrowser } = await import('./directoryBrowser')
const { selectedMedia } = await import('../state')

const tick = () => new Promise((r) => setTimeout(r, 0))

async function mount(): Promise<HTMLElement> {
  const root = document.createElement('div')
  document.body.appendChild(root)
  mountDirectoryBrowser(root)
  for (let i = 0; i < 20 && !root.querySelector('.entry-media'); i++) await tick()
  return root
}

describe('directoryBrowser select all', () => {
  beforeEach(() => {
    document.body.innerHTML = ''
    localStorage.clear()
    selectedMedia.set([])
    mocks.browseError = null
    mocks.browseCalls = []
  })

  it('selects every media file in the folder (not the subfolder) and unselects them again', async () => {
    const root = await mount()
    const all = root.querySelector<HTMLInputElement>('.select-all-checkbox')!
    expect(all.disabled).toBe(false)
    expect(all.checked).toBe(false)

    all.checked = true
    all.dispatchEvent(new Event('change', { bubbles: true }))
    expect(selectedMedia.get().map((e) => e.name).sort()).toEqual(['a.mp4', 'b.mp4', 'c.mp3'])
    expect(root.querySelectorAll('.entry-media.selected').length).toBe(3)
    expect(all.checked).toBe(true)

    all.checked = false
    all.dispatchEvent(new Event('change', { bubbles: true }))
    expect(selectedMedia.get()).toEqual([])
  })

  it('shows a partial selection as indeterminate and keeps files from other folders', async () => {
    const root = await mount()
    const all = root.querySelector<HTMLInputElement>('.select-all-checkbox')!
    const elsewhere = { ...entry('z.mp4', 'video'), path: 'D:\\other\\z.mp4' }
    selectedMedia.set([elsewhere, listing.entries[1]!])
    expect(all.indeterminate).toBe(true)
    expect(all.checked).toBe(false)

    all.checked = true
    all.dispatchEvent(new Event('change', { bubbles: true }))
    expect(selectedMedia.get().length).toBe(4) // the 3 here plus the other folder's
    expect(all.indeterminate).toBe(false)

    all.checked = false
    all.dispatchEvent(new Event('change', { bubbles: true }))
    expect(selectedMedia.get()).toEqual([elsewhere])
  })

  it('gives folder rows a real button and keeps the list semantics', async () => {
    const root = await mount()
    const folder = root.querySelector<HTMLElement>('.entry-dir')!
    expect(folder.getAttribute('role')).toBeNull()
    expect(folder.tabIndex).toBe(-1)
    const btn = folder.querySelector('button.row-btn')
    expect(btn).not.toBeNull()
    expect(btn!.textContent).toContain('sub')
    // media rows rely on their checkbox instead
    expect(root.querySelector('.entry-media button')).toBeNull()
  })

  it('opens a folder when its button is clicked and keeps keyboard focus in the list', async () => {
    const root = await mount()
    const btn = root.querySelector<HTMLButtonElement>('.entry-dir button.row-btn')!
    btn.focus()
    expect(document.activeElement).toBe(btn)
    mocks.browseCalls = []
    btn.click()
    for (let i = 0; i < 20 && mocks.browseCalls.length === 0; i++) await tick()
    await tick()
    expect(mocks.browseCalls).toEqual(['F:\\v\\sub'])
    // The list was rebuilt; focus must not have fallen back to <body>.
    expect(root.querySelector('.entry-list')!.contains(document.activeElement)).toBe(true)
  })

  it('stops select-all from acting on rows that are no longer shown after a failed listing', async () => {
    const root = await mount()
    const all = root.querySelector<HTMLInputElement>('.select-all-checkbox')!
    expect(all.disabled).toBe(false)
    mocks.browseError = new Error('boom')
    root.querySelector<HTMLButtonElement>('.refresh-btn')!.click()
    for (let i = 0; i < 20 && !root.querySelector('.entry-error'); i++) await tick()
    expect(root.querySelector('.entry-error')).not.toBeNull()
    expect(all.disabled).toBe(true)
  })
})
