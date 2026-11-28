import { api, type BrowseKind, type Entry } from '../api'
import { delegateActivate, errMsg, escapeHtml, requireEl, rowButton } from '../dom'
import { createEntryLabel, driveFor, keepFocusInList, renderListEmpty, renderListError, Sequencer } from './listing'

// Pickers only ever browse for an executable or a directory; the media picker
// (directoryBrowser.ts) is separate.
export type PickerKind = Extract<BrowseKind, 'exe' | 'dir'>

export interface DirPickerOpenOptions {
  // 'exe': click a file to select it. 'dir': an explicit "Select This
  // Directory" button selects whatever directory is being browsed.
  kind: PickerKind
  onSelect: (path: string) => void
  // Directory (or, for 'exe', a file path's parent) to start in; falls back
  // to the first drive if it can't be listed.
  initialPath?: string
}

export interface DirPickerHandle {
  open(options: DirPickerOpenOptions): Promise<void>
  close(): void
  isOpen(): boolean
}

// mountDirPicker renders the inline drive-select + breadcrumb + entry-list
// picker shared by settingsPanel (kind 'exe'/'dir') and progressPanel's Save
// picker (kind 'dir'). It is hidden until open() is called; the select
// callback is supplied per open() so the caller can capture whatever context
// (which settings field, which job) applied when the picker was opened.
//
// Keyboard: opening moves focus into the picker, Escape closes it, and closing
// returns focus to whatever opened it.
export function mountDirPicker(mount: HTMLElement): DirPickerHandle {
  const picker = document.createElement('div')
  picker.className = 'settings-picker'
  picker.hidden = true
  picker.setAttribute('role', 'dialog')
  picker.innerHTML = `
    <div class="browser-toolbar">
      <select class="drive-select" aria-label="Drive"></select>
    </div>
    <div class="breadcrumb">
      <button type="button" class="btn-link picker-up" hidden>.. Up</button>
      <span class="current-path"></span>
    </div>
    <ul class="entry-list picker-entry-list"></ul>
    <div class="settings-picker-actions">
      <button type="button" class="btn-primary picker-select-dir" hidden disabled>Select This Directory</button>
      <button type="button" class="btn-link picker-cancel">Cancel</button>
    </div>
  `
  mount.appendChild(picker)

  const driveSelect = requireEl<HTMLSelectElement>(picker, '.drive-select')
  const upBtn = requireEl<HTMLButtonElement>(picker, '.picker-up')
  const currentPathEl = requireEl<HTMLSpanElement>(picker, '.current-path')
  const entryList = requireEl<HTMLUListElement>(picker, '.picker-entry-list')
  const selectDirBtn = requireEl<HTMLButtonElement>(picker, '.picker-select-dir')
  const cancelBtn = requireEl<HTMLButtonElement>(picker, '.picker-cancel')

  let current: DirPickerOpenOptions | null = null
  // The directory currently listed successfully; '' after a failed
  // navigation so a stale path can never be confirmed.
  let selectedDir = ''
  // The rows currently shown, by path, for the delegated handler.
  let entriesByPath = new Map<string, Entry>()
  // The parent of the directory being shown, for the Up button.
  let parentDir: string | undefined
  // A slower, earlier response never overwrites a newer one.
  const navSeq = new Sequencer()
  // Bumped by open()/close(); lets open() notice it was superseded mid-await.
  let session = 0
  // Who had focus when the picker opened, to hand focus back on close.
  let opener: HTMLElement | null = null

  // 'superseded' means a newer navigation or open()/close() took over while
  // this one was in flight; it is neither a success nor a failure to report.
  type NavResult = 'ok' | 'failed' | 'superseded'

  async function navigate(path: string): Promise<NavResult> {
    const ticket = navSeq.next()
    const kind = current?.kind ?? 'dir'
    try {
      const listing = await api.browse(path, kind)
      if (!navSeq.isCurrent(ticket)) return 'superseded'
      selectedDir = listing.path
      selectDirBtn.disabled = false
      currentPathEl.textContent = listing.path
      upBtn.hidden = !listing.parent
      parentDir = listing.parent
      renderEntries(listing.entries)
      return 'ok'
    } catch (err) {
      if (!navSeq.isCurrent(ticket)) return 'superseded'
      selectedDir = ''
      selectDirBtn.disabled = true
      currentPathEl.textContent = ''
      upBtn.hidden = true
      parentDir = undefined
      entriesByPath = new Map()
      const hadFocus = entryList.contains(document.activeElement)
      renderListError(entryList, errMsg(err))
      keepFocusInList(entryList, hadFocus)
      return 'failed'
    }
  }

  function select(path: string): void {
    const cb = current?.onSelect
    close()
    if (path && cb) cb(path)
  }

  function renderEntries(entries: Entry[]): void {
    entriesByPath = new Map(entries.map((e) => [e.path, e]))
    const hadFocus = entryList.contains(document.activeElement)
    entryList.innerHTML = ''
    if (entries.length === 0) {
      renderListEmpty(entryList)
      keepFocusInList(entryList, hadFocus)
      return
    }
    const rows = document.createDocumentFragment()
    for (const entry of entries) {
      const li = document.createElement('li')
      li.className = `entry entry-${entry.type}`
      li.dataset.path = entry.path
      const btn = rowButton()
      btn.appendChild(createEntryLabel(entry))
      li.appendChild(btn)
      rows.appendChild(li)
    }
    entryList.appendChild(rows)
    keepFocusInList(entryList, hadFocus)
  }

  function close(): void {
    navSeq.invalidate() // drop any in-flight navigation
    session++
    const wasOpen = !picker.hidden
    picker.hidden = true
    current = null
    selectedDir = ''
    if (wasOpen && opener && opener.isConnected) opener.focus()
    opener = null
  }

  async function open(options: DirPickerOpenOptions): Promise<void> {
    navSeq.invalidate()
    const mySession = ++session
    current = options
    selectedDir = ''
    if (picker.hidden) opener = document.activeElement instanceof HTMLElement ? document.activeElement : null
    picker.hidden = false
    picker.setAttribute('aria-label', options.kind === 'dir' ? 'Choose a folder' : 'Choose a file')
    selectDirBtn.hidden = options.kind !== 'dir'
    selectDirBtn.disabled = true
    currentPathEl.textContent = ''
    upBtn.hidden = true
    entryList.innerHTML = ''
    driveSelect.focus()

    let drives: Awaited<ReturnType<typeof api.drives>>
    try {
      drives = await api.drives()
    } catch (err) {
      if (mySession === session) renderListError(entryList, errMsg(err))
      return
    }
    if (mySession !== session) return

    driveSelect.innerHTML = drives
      .map((d) => `<option value="${escapeHtml(d.name)}">${escapeHtml(d.name)} ${escapeHtml(d.label)}</option>`)
      .join('')
    const firstDrive = drives[0]
    if (!firstDrive) {
      renderListError(entryList, 'No drives found')
      return
    }

    const initial = options.initialPath?.trim()
    if (initial) {
      const match = driveFor(drives, initial)
      if (match) driveSelect.value = match.name
      // Only a real failure falls back to the first drive; if the user (or a
      // close/reopen) moved on meanwhile, leave their navigation alone.
      if ((await navigate(initial)) !== 'failed') return
      if (mySession !== session) return
      driveSelect.value = firstDrive.name
    }
    await navigate(firstDrive.name)
  }

  delegateActivate(entryList, '.entry', (li) => {
    const entry = li.dataset.path ? entriesByPath.get(li.dataset.path) : undefined
    if (!entry) return
    if (entry.type === 'dir') void navigate(entry.path)
    else select(entry.path)
  })
  upBtn.addEventListener('click', () => {
    if (parentDir) void navigate(parentDir)
  })
  driveSelect.addEventListener('change', () => navigate(driveSelect.value))
  selectDirBtn.addEventListener('click', () => select(selectedDir))
  cancelBtn.addEventListener('click', close)
  picker.addEventListener('keydown', (e) => {
    if (e.key === 'Escape') {
      e.stopPropagation()
      close()
    }
  })

  return { open, close, isOpen: () => !picker.hidden }
}
