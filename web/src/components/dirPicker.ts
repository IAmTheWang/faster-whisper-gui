import { api, type BrowseKind, type Entry } from '../api'
import { errMsg, escapeHtml, makeActivatable } from '../dom'

export interface DirPickerOpenOptions {
  // 'exe': click a file to select it. 'dir': an explicit "Select This
  // Directory" button selects whatever directory is being browsed.
  kind: Extract<BrowseKind, 'exe' | 'dir'>
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
export function mountDirPicker(mount: HTMLElement): DirPickerHandle {
  const picker = document.createElement('div')
  picker.className = 'settings-picker'
  picker.hidden = true
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

  const driveSelect = picker.querySelector<HTMLSelectElement>('.drive-select')!
  const upBtn = picker.querySelector<HTMLButtonElement>('.picker-up')!
  const currentPathEl = picker.querySelector<HTMLSpanElement>('.current-path')!
  const entryList = picker.querySelector<HTMLUListElement>('.picker-entry-list')!
  const selectDirBtn = picker.querySelector<HTMLButtonElement>('.picker-select-dir')!
  const cancelBtn = picker.querySelector<HTMLButtonElement>('.picker-cancel')!

  let current: DirPickerOpenOptions | null = null
  // The directory currently listed successfully; '' after a failed
  // navigation so a stale path can never be confirmed.
  let selectedDir = ''
  // Monotonic counter: a slower, earlier response never overwrites a newer one.
  let seq = 0
  // Bumped by open()/close(); lets open() notice it was superseded mid-await.
  let session = 0

  function showError(message: string): void {
    entryList.innerHTML = `<li class="entry-error" role="alert">${escapeHtml(message)}</li>`
  }

  // 'superseded' means a newer navigation or open()/close() took over while
  // this one was in flight; it is neither a success nor a failure to report.
  type NavResult = 'ok' | 'failed' | 'superseded'

  async function navigate(path: string): Promise<NavResult> {
    const mySeq = ++seq
    const kind = current?.kind ?? 'dir'
    try {
      const listing = await api.browse(path, kind)
      if (mySeq !== seq) return 'superseded'
      selectedDir = listing.path
      selectDirBtn.disabled = false
      currentPathEl.textContent = listing.path
      upBtn.hidden = !listing.parent
      upBtn.onclick = () => navigate(listing.parent!)
      renderEntries(listing.entries)
      return 'ok'
    } catch (err) {
      if (mySeq !== seq) return 'superseded'
      selectedDir = ''
      selectDirBtn.disabled = true
      currentPathEl.textContent = ''
      upBtn.hidden = true
      upBtn.onclick = null
      showError(errMsg(err))
      return 'failed'
    }
  }

  function select(path: string): void {
    const cb = current?.onSelect
    close()
    if (path && cb) cb(path)
  }

  function renderEntries(entries: Entry[]): void {
    entryList.innerHTML = ''
    if (entries.length === 0) {
      entryList.innerHTML = '<li class="entry-empty">(empty)</li>'
      return
    }
    for (const entry of entries) {
      const li = document.createElement('li')
      li.className = `entry entry-${entry.type}`
      const icon = entry.type === 'dir' ? '📁' : '📄'
      li.textContent = `${icon} ${entry.name}`
      makeActivatable(li, () => {
        if (entry.type === 'dir') {
          navigate(entry.path)
        } else {
          select(entry.path)
        }
      })
      entryList.appendChild(li)
    }
  }

  function close(): void {
    seq++ // drop any in-flight navigation
    session++
    picker.hidden = true
    current = null
    selectedDir = ''
  }

  async function open(options: DirPickerOpenOptions): Promise<void> {
    seq++
    const mySession = ++session
    current = options
    selectedDir = ''
    picker.hidden = false
    selectDirBtn.hidden = options.kind !== 'dir'
    selectDirBtn.disabled = true
    currentPathEl.textContent = ''
    upBtn.hidden = true
    entryList.innerHTML = ''

    let drives: Awaited<ReturnType<typeof api.drives>>
    try {
      drives = await api.drives()
    } catch (err) {
      if (mySession === session) showError(errMsg(err))
      return
    }
    if (mySession !== session) return

    driveSelect.innerHTML = drives
      .map((d) => `<option value="${escapeHtml(d.name)}">${escapeHtml(d.name)} ${escapeHtml(d.label)}</option>`)
      .join('')
    if (drives.length === 0) {
      showError('No drives found')
      return
    }

    const initial = options.initialPath?.trim()
    if (initial) {
      const match = drives.find((d) => initial.toUpperCase().startsWith(d.name.toUpperCase()))
      if (match) driveSelect.value = match.name
      // Only a real failure falls back to the first drive; if the user (or a
      // close/reopen) moved on meanwhile, leave their navigation alone.
      if ((await navigate(initial)) !== 'failed') return
      if (mySession !== session) return
      driveSelect.value = drives[0].name
    }
    await navigate(drives[0].name)
  }

  driveSelect.addEventListener('change', () => navigate(driveSelect.value))
  selectDirBtn.addEventListener('click', () => select(selectedDir))
  cancelBtn.addEventListener('click', close)

  return { open, close, isOpen: () => !picker.hidden }
}
