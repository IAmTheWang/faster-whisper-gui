import { api, type Entry } from '../api'
import {
  delegateActivate,
  dirnameOf,
  errMsg,
  markActivatable,
  requireEl,
  stripSurroundingQuotes,
} from '../dom'
import { selectedMedia } from '../state'
import {
  compareEntries,
  defaultDirFor,
  isSortKey,
  loadStoredSort,
  SORT_KEYS,
  SORT_LABELS,
  storeSort,
  type SortDir,
  type SortKey,
} from './entrySort'
import { createEntryLabel, renderListEmpty, renderListError, Sequencer } from './listing'

function toggleSelection(entry: Entry): void {
  const current = selectedMedia.get()
  const idx = current.findIndex((e) => e.path === entry.path)
  if (idx === -1) {
    selectedMedia.set([...current, entry])
  } else {
    selectedMedia.set([...current.slice(0, idx), ...current.slice(idx + 1)])
  }
}

export function mountDirectoryBrowser(root: HTMLElement): void {
  root.innerHTML = `
    <div class="panel">
      <h2>Select Media</h2>
      <div class="browser-toolbar">
        <input type="text" class="drive-select" aria-label="Drive or folder path"
               placeholder="Drive letter or paste a full path" autocomplete="off" spellcheck="false" />
      </div>
      <div class="selection-bar">
        <span class="selection-count"></span>
        <button type="button" class="btn-link clear-selection" hidden>Clear</button>
      </div>
      <div class="listing-header">
        <div class="breadcrumb"></div>
        <div class="header-actions">
          <div class="sort-controls">
            ${SORT_KEYS.map(
              (key) =>
                `<button type="button" class="sort-option" data-key="${key}" aria-pressed="false">${SORT_LABELS[key]}<span class="sort-arrow" aria-hidden="true"></span></button>`,
            ).join('')}
          </div>
          <button type="button" class="btn-link refresh-btn" aria-label="Refresh" title="Refresh" disabled>⟳</button>
        </div>
      </div>
      <ul class="entry-list"></ul>
    </div>
  `

  const driveInput = requireEl<HTMLInputElement>(root, '.drive-select')
  const sortControls = requireEl<HTMLDivElement>(root, '.sort-controls')
  const refreshBtn = requireEl<HTMLButtonElement>(root, '.refresh-btn')
  const selectionCount = requireEl<HTMLSpanElement>(root, '.selection-count')
  const clearSelectionBtn = requireEl<HTMLButtonElement>(root, '.clear-selection')
  const breadcrumb = requireEl<HTMLDivElement>(root, '.breadcrumb')
  const entryList = requireEl<HTMLUListElement>(root, '.entry-list')

  let currentEntries: Entry[] = []
  let currentPath = ''
  // The rows currently shown, by path: the delegated handlers look entries up
  // here instead of every row closing over its own.
  let entriesByPath = new Map<string, Entry>()
  const storedSort = loadStoredSort(localStorage)
  let sortKey: SortKey = storedSort?.key ?? 'name'
  let sortDir: SortDir = storedSort?.dir ?? 'asc'

  function updateSortControlsUI(): void {
    sortControls.querySelectorAll<HTMLButtonElement>('.sort-option').forEach((btn) => {
      const key = btn.dataset.key
      if (!isSortKey(key)) return
      const isActive = key === sortKey
      const arrow = requireEl<HTMLSpanElement>(btn, '.sort-arrow')
      btn.classList.toggle('active', isActive)
      btn.setAttribute('aria-pressed', String(isActive))
      if (isActive) {
        const ascending = sortDir === 'asc'
        btn.setAttribute('aria-label', `Sort by ${SORT_LABELS[key]}, ${ascending ? 'ascending' : 'descending'}`)
        arrow.textContent = ascending ? '↑' : '↓'
      } else {
        btn.setAttribute('aria-label', `Sort by ${SORT_LABELS[key]}`)
        arrow.textContent = ''
      }
    })
  }

  clearSelectionBtn.addEventListener('click', () => selectedMedia.set([]))

  async function loadDrives(): Promise<void> {
    const drives = await api.drives()
    const firstDrive = drives[0]
    if (!firstDrive) {
      renderListError(entryList, 'No drives found')
      return
    }

    const settings = await api.settings().catch(() => null)
    const defaultDir = settings?.defaultVideoDir
    if (defaultDir && (await navigate(defaultDir)) === null) return
    // No default directory, or it can't be listed (moved, drive unplugged):
    // start at the first drive rather than leaving the user on an error.
    await navigate(firstDrive.name)
  }

  // A slower, earlier response must never overwrite a newer navigation's listing.
  const navSeq = new Sequencer()

  async function navigate(path: string): Promise<Error | null> {
    const ticket = navSeq.next()
    try {
      const listing = await api.browse(path)
      if (!navSeq.isCurrent(ticket)) return null
      currentPath = listing.path
      driveInput.value = listing.path
      refreshBtn.disabled = false
      renderBreadcrumb(listing.path, listing.parent)
      renderEntries(listing.entries)
      return null
    } catch (err) {
      if (!navSeq.isCurrent(ticket)) return null
      const error = err instanceof Error ? err : new Error(String(err))
      renderListError(entryList, error.message)
      driveInput.value = currentPath
      return error
    }
  }

  async function navigateToPastedPath(raw: string): Promise<void> {
    const cleaned = stripSurroundingQuotes(raw)
    if (!cleaned || cleaned === currentPath) return
    const err = await navigate(cleaned)
    if (err && /is not a directory/i.test(err.message || '')) {
      const parent = dirnameOf(cleaned)
      if (parent) await navigate(parent)
    }
  }

  function renderBreadcrumb(path: string, parent?: string): void {
    breadcrumb.innerHTML = ''
    if (parent) {
      const up = document.createElement('button')
      up.type = 'button'
      up.textContent = '.. Up'
      up.className = 'btn-link'
      up.addEventListener('click', () => navigate(parent))
      breadcrumb.appendChild(up)
    }
    const span = document.createElement('span')
    span.className = 'current-path'
    span.textContent = path
    breadcrumb.appendChild(span)
  }

  function renderEntries(entries: Entry[]): void {
    currentEntries = entries
    renderSortedEntries()
  }

  function renderSortedEntries(): void {
    const entries = [...currentEntries].sort(compareEntries(sortKey, sortDir))
    entriesByPath = new Map(entries.map((e) => [e.path, e]))
    entryList.innerHTML = ''
    if (entries.length === 0) {
      renderListEmpty(entryList)
      return
    }

    const selectedPaths = new Set(selectedMedia.get().map((e) => e.path))
    const rows = document.createDocumentFragment()
    for (const entry of entries) {
      const li = document.createElement('li')
      li.className = `entry entry-${entry.type}`
      li.dataset.path = entry.path

      if (entry.type === 'video' || entry.type === 'audio') {
        li.classList.add('entry-media')
        const checkbox = document.createElement('input')
        checkbox.type = 'checkbox'
        checkbox.className = 'media-checkbox'
        checkbox.setAttribute('aria-label', `Select ${entry.name}`)
        checkbox.checked = selectedPaths.has(entry.path)
        li.appendChild(checkbox)
        if (checkbox.checked) li.classList.add('selected')
      } else {
        // The checkbox is the focusable control on media rows (native
        // semantics); a folder row is itself the button.
        markActivatable(li)
      }
      li.appendChild(createEntryLabel(entry))
      rows.appendChild(li)
    }
    entryList.appendChild(rows)
  }

  // One set of listeners for the whole list, however many rows it has. A click
  // on a media row's checkbox is left to its own change event so it can't
  // toggle twice.
  delegateActivate(
    entryList,
    '.entry',
    (li) => {
      const entry = li.dataset.path ? entriesByPath.get(li.dataset.path) : undefined
      if (!entry) return
      if (entry.type === 'dir') void navigate(entry.path)
      else toggleSelection(entry)
    },
    '.media-checkbox',
  )
  entryList.addEventListener('change', (e) => {
    const target = e.target as HTMLElement
    if (!target.matches('.media-checkbox')) return
    const path = target.closest<HTMLElement>('.entry')?.dataset.path
    const entry = path ? entriesByPath.get(path) : undefined
    if (entry) toggleSelection(entry)
  })

  driveInput.addEventListener('paste', () => setTimeout(() => navigateToPastedPath(driveInput.value), 0))
  driveInput.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') {
      e.preventDefault()
      navigateToPastedPath(driveInput.value)
    }
  })
  driveInput.addEventListener('change', () => navigateToPastedPath(driveInput.value))
  driveInput.addEventListener('focus', () => driveInput.select())

  sortControls.addEventListener('click', (e) => {
    const btn = (e.target as HTMLElement).closest<HTMLButtonElement>('.sort-option')
    const key = btn?.dataset.key
    if (!isSortKey(key)) return
    if (key === sortKey) {
      sortDir = sortDir === 'asc' ? 'desc' : 'asc'
    } else {
      sortKey = key
      sortDir = defaultDirFor(key)
    }
    storeSort(localStorage, sortKey, sortDir)
    updateSortControlsUI()
    renderSortedEntries()
  })

  refreshBtn.addEventListener('click', async () => {
    if (!currentPath) return
    const hadFocus = document.activeElement === refreshBtn
    refreshBtn.disabled = true
    refreshBtn.classList.add('is-refreshing')
    try {
      await navigate(currentPath)
    } finally {
      refreshBtn.disabled = false
      refreshBtn.classList.remove('is-refreshing')
      if (hadFocus) refreshBtn.focus()
    }
  })

  selectedMedia.subscribe((media) => {
    const selectedPaths = new Set(media.map((v) => v.path))
    entryList.querySelectorAll<HTMLLIElement>('.entry-media').forEach((el) => {
      const isSelected = !!el.dataset.path && selectedPaths.has(el.dataset.path)
      el.classList.toggle('selected', isSelected)
      const checkbox = el.querySelector<HTMLInputElement>('.media-checkbox')
      if (checkbox) checkbox.checked = isSelected
    })
    selectionCount.textContent = media.length > 0 ? `${media.length} selected` : ''
    clearSelectionBtn.hidden = media.length === 0
  })

  updateSortControlsUI()
  loadDrives().catch((err) => renderListError(entryList, `Could not load drives: ${errMsg(err)}`))
}
