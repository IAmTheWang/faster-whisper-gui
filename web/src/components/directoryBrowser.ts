import { api, type Entry } from '../api'
import { escapeHtml } from '../dom'
import { selectedMedia } from '../state'

type SortKey = 'name' | 'type' | 'modTime' | 'createdTime'
type SortDir = 'asc' | 'desc'

const SORT_KEYS: SortKey[] = ['name', 'type', 'modTime', 'createdTime']
const SORT_LABELS: Record<SortKey, string> = {
  name: 'Name',
  type: 'Type',
  modTime: 'Modified',
  createdTime: 'Created',
}
const SORT_STORAGE_KEY = 'directoryBrowser.sort'

function defaultDirFor(key: SortKey): SortDir {
  return key === 'modTime' || key === 'createdTime' ? 'desc' : 'asc'
}

function loadStoredSort(): { key: SortKey; dir: SortDir } | null {
  try {
    const raw = localStorage.getItem(SORT_STORAGE_KEY)
    if (!raw) return null
    const parsed = JSON.parse(raw) as { key?: string; dir?: string }
    if (!parsed.key || !SORT_KEYS.includes(parsed.key as SortKey)) return null
    if (parsed.dir !== 'asc' && parsed.dir !== 'desc') return null
    return { key: parsed.key as SortKey, dir: parsed.dir }
  } catch {
    return null
  }
}

function storeSort(key: SortKey, dir: SortDir): void {
  try {
    localStorage.setItem(SORT_STORAGE_KEY, JSON.stringify({ key, dir }))
  } catch {
    // ignore (e.g. private browsing quota)
  }
}

function compareByName(a: Entry, b: Entry): number {
  const nameA = a.name.toLowerCase()
  const nameB = b.name.toLowerCase()
  return nameA < nameB ? -1 : nameA > nameB ? 1 : 0
}

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
        <select class="drive-select"></select>
      </div>
      <div class="selection-bar">
        <span class="selection-count"></span>
        <button type="button" class="btn-link clear-selection" hidden>Clear</button>
      </div>
      <div class="listing-header">
        <div class="breadcrumb"></div>
        <div class="sort-controls">
          ${SORT_KEYS.map(
            (key) =>
              `<button type="button" class="sort-option" data-key="${key}">${SORT_LABELS[key]}<span class="sort-arrow" aria-hidden="true"></span></button>`,
          ).join('')}
        </div>
        <button type="button" class="btn-link refresh-btn" aria-label="Refresh" title="Refresh">⟳</button>
      </div>
      <ul class="entry-list"></ul>
    </div>
  `

  const driveSelect = root.querySelector<HTMLSelectElement>('.drive-select')!
  const sortControls = root.querySelector<HTMLDivElement>('.sort-controls')!
  const refreshBtn = root.querySelector<HTMLButtonElement>('.refresh-btn')!
  const selectionCount = root.querySelector<HTMLSpanElement>('.selection-count')!
  const clearSelectionBtn = root.querySelector<HTMLButtonElement>('.clear-selection')!
  const breadcrumb = root.querySelector<HTMLDivElement>('.breadcrumb')!
  const entryList = root.querySelector<HTMLUListElement>('.entry-list')!

  let currentEntries: Entry[] = []
  let currentPath = ''
  const storedSort = loadStoredSort()
  let sortKey: SortKey = storedSort?.key ?? 'name'
  let sortDir: SortDir = storedSort?.dir ?? 'asc'

  function compareEntries(a: Entry, b: Entry): number {
    if (a.type === 'dir' && b.type !== 'dir') return -1
    if (a.type !== 'dir' && b.type === 'dir') return 1

    let cmp: number
    switch (sortKey) {
      case 'type':
        cmp = a.type < b.type ? -1 : a.type > b.type ? 1 : compareByName(a, b)
        break
      case 'modTime':
        cmp = a.modTime - b.modTime || compareByName(a, b)
        break
      case 'createdTime':
        cmp = a.createdTime - b.createdTime || compareByName(a, b)
        break
      default:
        cmp = compareByName(a, b)
    }
    return sortDir === 'asc' ? cmp : -cmp
  }

  function updateSortControlsUI(): void {
    sortControls.querySelectorAll<HTMLButtonElement>('.sort-option').forEach((btn) => {
      const key = btn.dataset.key as SortKey
      const isActive = key === sortKey
      const arrow = btn.querySelector<HTMLSpanElement>('.sort-arrow')!
      btn.classList.toggle('active', isActive)
      if (isActive) {
        const ascending = sortDir === 'asc'
        btn.setAttribute('aria-sort', ascending ? 'ascending' : 'descending')
        btn.setAttribute('aria-label', `Sort by ${SORT_LABELS[key]}, ${ascending ? 'ascending' : 'descending'}`)
        arrow.textContent = ascending ? '↑' : '↓'
      } else {
        btn.removeAttribute('aria-sort')
        btn.setAttribute('aria-label', `Sort by ${SORT_LABELS[key]}`)
        arrow.textContent = ''
      }
    })
  }

  clearSelectionBtn.addEventListener('click', () => selectedMedia.set([]))

  async function loadDrives(): Promise<void> {
    const drives = await api.drives()
    driveSelect.innerHTML = drives
      .map((d) => `<option value="${escapeHtml(d.name)}">${escapeHtml(d.name)} ${escapeHtml(d.label)}</option>`)
      .join('')
    if (drives.length === 0) return

    const settings = await api.settings().catch(() => null)
    const defaultDir = settings?.defaultVideoDir
    const startDrive = defaultDir ? drives.find((d) => defaultDir.toUpperCase().startsWith(d.name.toUpperCase())) : undefined
    driveSelect.value = (startDrive ?? drives[0]).name
    await navigate(defaultDir || drives[0].name)
  }

  async function navigate(path: string): Promise<void> {
    try {
      const listing = await api.browse(path)
      currentPath = listing.path
      renderBreadcrumb(listing.path, listing.parent)
      renderEntries(listing.entries)
    } catch (err) {
      entryList.innerHTML = `<li class="entry-error">${escapeHtml(err instanceof Error ? err.message : String(err))}</li>`
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
    const entries = [...currentEntries].sort(compareEntries)
    entryList.innerHTML = ''
    if (entries.length === 0) {
      entryList.innerHTML = '<li class="entry-empty">(empty)</li>'
      return
    }

    const selectedPaths = new Set(selectedMedia.get().map((e) => e.path))
    for (const entry of entries) {
      const li = document.createElement('li')
      li.className = `entry entry-${entry.type}`
      li.dataset.path = entry.path

      const isMedia = entry.type === 'video' || entry.type === 'audio'
      if (isMedia) {
        li.classList.add('entry-media')
        const checkbox = document.createElement('input')
        checkbox.type = 'checkbox'
        checkbox.className = 'media-checkbox'
        checkbox.checked = selectedPaths.has(entry.path)
        checkbox.addEventListener('click', (e) => e.stopPropagation())
        checkbox.addEventListener('change', () => toggleSelection(entry))
        li.appendChild(checkbox)
        if (checkbox.checked) li.classList.add('selected')
      }

      const label = document.createElement('span')
      const icon = entry.type === 'dir' ? '📁' : entry.type === 'audio' ? '🎵' : '🎬'
      label.textContent = `${icon} ${entry.name}`
      li.appendChild(label)

      li.addEventListener('click', () => {
        if (entry.type === 'dir') {
          navigate(entry.path)
        } else {
          toggleSelection(entry)
        }
      })
      entryList.appendChild(li)
    }
  }

  driveSelect.addEventListener('change', () => navigate(driveSelect.value))

  sortControls.addEventListener('click', (e) => {
    const btn = (e.target as HTMLElement).closest<HTMLButtonElement>('.sort-option')
    if (!btn) return
    const key = btn.dataset.key as SortKey
    if (key === sortKey) {
      sortDir = sortDir === 'asc' ? 'desc' : 'asc'
    } else {
      sortKey = key
      sortDir = defaultDirFor(key)
    }
    storeSort(sortKey, sortDir)
    updateSortControlsUI()
    renderSortedEntries()
  })

  refreshBtn.addEventListener('click', async () => {
    if (!currentPath) return
    refreshBtn.disabled = true
    refreshBtn.classList.add('is-refreshing')
    try {
      await navigate(currentPath)
    } finally {
      refreshBtn.disabled = false
      refreshBtn.classList.remove('is-refreshing')
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
  loadDrives()
}
