import { api, type Entry } from '../api'
import { escapeHtml } from '../dom'
import { selectedVideos } from '../state'

function toggleSelection(entry: Entry): void {
  const current = selectedVideos.get()
  const idx = current.findIndex((e) => e.path === entry.path)
  if (idx === -1) {
    selectedVideos.set([...current, entry])
  } else {
    selectedVideos.set([...current.slice(0, idx), ...current.slice(idx + 1)])
  }
}

export function mountDirectoryBrowser(root: HTMLElement): void {
  root.innerHTML = `
    <div class="panel">
      <h2>Select Video</h2>
      <div class="browser-toolbar">
        <select class="drive-select"></select>
      </div>
      <div class="selection-bar">
        <span class="selection-count"></span>
        <button type="button" class="btn-link clear-selection" hidden>Clear</button>
      </div>
      <div class="breadcrumb"></div>
      <ul class="entry-list"></ul>
    </div>
  `

  const driveSelect = root.querySelector<HTMLSelectElement>('.drive-select')!
  const selectionCount = root.querySelector<HTMLSpanElement>('.selection-count')!
  const clearSelectionBtn = root.querySelector<HTMLButtonElement>('.clear-selection')!
  const breadcrumb = root.querySelector<HTMLDivElement>('.breadcrumb')!
  const entryList = root.querySelector<HTMLUListElement>('.entry-list')!

  clearSelectionBtn.addEventListener('click', () => selectedVideos.set([]))

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
    entryList.innerHTML = ''
    if (entries.length === 0) {
      entryList.innerHTML = '<li class="entry-empty">(empty)</li>'
      return
    }

    const selectedPaths = new Set(selectedVideos.get().map((e) => e.path))
    for (const entry of entries) {
      const li = document.createElement('li')
      li.className = `entry entry-${entry.type}`
      li.dataset.path = entry.path

      if (entry.type === 'video') {
        const checkbox = document.createElement('input')
        checkbox.type = 'checkbox'
        checkbox.className = 'video-checkbox'
        checkbox.checked = selectedPaths.has(entry.path)
        checkbox.addEventListener('click', (e) => e.stopPropagation())
        checkbox.addEventListener('change', () => toggleSelection(entry))
        li.appendChild(checkbox)
        if (checkbox.checked) li.classList.add('selected')
      }

      const label = document.createElement('span')
      const icon = entry.type === 'dir' ? '📁' : '🎬'
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

  selectedVideos.subscribe((videos) => {
    const selectedPaths = new Set(videos.map((v) => v.path))
    entryList.querySelectorAll<HTMLLIElement>('.entry-video').forEach((el) => {
      const isSelected = !!el.dataset.path && selectedPaths.has(el.dataset.path)
      el.classList.toggle('selected', isSelected)
      const checkbox = el.querySelector<HTMLInputElement>('.video-checkbox')
      if (checkbox) checkbox.checked = isSelected
    })
    selectionCount.textContent = videos.length > 0 ? `${videos.length} selected` : ''
    clearSelectionBtn.hidden = videos.length === 0
  })

  loadDrives()
}
