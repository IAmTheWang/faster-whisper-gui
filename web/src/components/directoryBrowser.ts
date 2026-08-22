import { api, type Entry } from '../api'
import { escapeHtml } from '../dom'
import { selectedVideo } from '../state'

export function mountDirectoryBrowser(root: HTMLElement): void {
  root.innerHTML = `
    <div class="panel">
      <h2>选择视频</h2>
      <div class="browser-toolbar">
        <select class="drive-select"></select>
      </div>
      <div class="breadcrumb"></div>
      <ul class="entry-list"></ul>
    </div>
  `

  const driveSelect = root.querySelector<HTMLSelectElement>('.drive-select')!
  const breadcrumb = root.querySelector<HTMLDivElement>('.breadcrumb')!
  const entryList = root.querySelector<HTMLUListElement>('.entry-list')!

  async function loadDrives(): Promise<void> {
    const drives = await api.drives()
    driveSelect.innerHTML = drives
      .map((d) => `<option value="${escapeHtml(d.name)}">${escapeHtml(d.name)} ${escapeHtml(d.label)}</option>`)
      .join('')
    if (drives.length > 0) {
      await navigate(drives[0].name)
    }
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
      up.textContent = '.. 上一级'
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
      entryList.innerHTML = '<li class="entry-empty">(空)</li>'
      return
    }

    const selected = selectedVideo.get()
    for (const entry of entries) {
      const li = document.createElement('li')
      li.className = `entry entry-${entry.type}`
      li.dataset.path = entry.path
      const icon = entry.type === 'dir' ? '📁' : '🎬'
      li.textContent = `${icon} ${entry.name}`
      if (entry.type === 'video' && selected?.path === entry.path) {
        li.classList.add('selected')
      }
      li.addEventListener('click', () => {
        if (entry.type === 'dir') {
          navigate(entry.path)
        } else {
          selectedVideo.set(entry)
        }
      })
      entryList.appendChild(li)
    }
  }

  driveSelect.addEventListener('change', () => navigate(driveSelect.value))

  selectedVideo.subscribe((video) => {
    entryList.querySelectorAll<HTMLLIElement>('.entry-video').forEach((el) => {
      el.classList.toggle('selected', !!video && el.dataset.path === video.path)
    })
  })

  loadDrives()
}
