import { api, subscribeJobEvents, type Entry, type JobStatus } from '../api'
import { escapeHtml } from '../dom'
import { activeTabId, openTabs } from '../state'

const stageLabels: Record<JobStatus, string> = {
  queued: 'Queued',
  extracting_audio: 'Extracting audio',
  transcribing: 'Transcribing',
  done: 'Done',
  failed: 'Failed',
  canceled: 'Canceled',
}

interface TabState {
  status: JobStatus
  percent: number
  message: string
  srtPath?: string
  error?: string
  unsubscribe: () => void
}

// mountProgressPanel keeps one TabState per open Progress tab, independent
// of which tab is frontmost — this is what lets a backgrounded job keep
// progressing (SSE stays connected) while another tab is being viewed.
export function mountProgressPanel(root: HTMLElement, onJobFinished: () => void): void {
  root.innerHTML = `
    <div class="panel">
      <h2>Progress</h2>
      <div class="progress-tabs"></div>
      <div class="progress-empty">No job started yet</div>
      <div class="progress-active" hidden>
        <div class="progress-bar"><div class="progress-bar-fill"></div></div>
        <div class="progress-stage"></div>
        <div class="progress-message"></div>
        <button type="button" class="btn-secondary cancel-btn">Cancel</button>
        <div class="progress-result"></div>
        <div class="progress-save-actions">
          <button type="button" class="btn-secondary save-btn">Save</button>
          <button type="button" class="btn-secondary save-all-btn">Save All Completed</button>
        </div>
        <div class="save-status"></div>
      </div>
      <div class="settings-picker" hidden>
        <div class="browser-toolbar">
          <select class="drive-select"></select>
        </div>
        <div class="breadcrumb">
          <button type="button" class="btn-link picker-up" hidden>.. Up</button>
          <span class="current-path"></span>
        </div>
        <ul class="entry-list picker-entry-list"></ul>
        <div class="settings-picker-actions">
          <button type="button" class="btn-primary picker-select-dir">Select This Directory</button>
          <button type="button" class="btn-link picker-cancel">Cancel</button>
        </div>
      </div>
    </div>
  `

  const tabStrip = root.querySelector<HTMLDivElement>('.progress-tabs')!
  const empty = root.querySelector<HTMLDivElement>('.progress-empty')!
  const active = root.querySelector<HTMLDivElement>('.progress-active')!
  const fill = root.querySelector<HTMLDivElement>('.progress-bar-fill')!
  const stageEl = root.querySelector<HTMLDivElement>('.progress-stage')!
  const messageEl = root.querySelector<HTMLDivElement>('.progress-message')!
  const cancelBtn = root.querySelector<HTMLButtonElement>('.cancel-btn')!
  const resultEl = root.querySelector<HTMLDivElement>('.progress-result')!
  const saveBtn = root.querySelector<HTMLButtonElement>('.save-btn')!
  const saveAllBtn = root.querySelector<HTMLButtonElement>('.save-all-btn')!
  const saveStatusEl = root.querySelector<HTMLDivElement>('.save-status')!

  const picker = root.querySelector<HTMLDivElement>('.settings-picker')!
  const driveSelect = picker.querySelector<HTMLSelectElement>('.drive-select')!
  const pickerUpBtn = picker.querySelector<HTMLButtonElement>('.picker-up')!
  const currentPathEl = picker.querySelector<HTMLSpanElement>('.current-path')!
  const pickerEntryList = picker.querySelector<HTMLUListElement>('.picker-entry-list')!
  const selectDirBtn = picker.querySelector<HTMLButtonElement>('.picker-select-dir')!
  const cancelPickerBtn = picker.querySelector<HTMLButtonElement>('.picker-cancel')!

  const jobStates = new Map<string, TabState>()
  let pickerMode: 'single' | 'all' | null = null
  let pickerPath = ''

  function updateSaveAllButton(): void {
    saveAllBtn.disabled = ![...jobStates.values()].some((s) => s.status === 'done')
  }

  function renderTabs(): void {
    const tabs = openTabs.get()
    const activeId = activeTabId.get()
    tabStrip.innerHTML = ''
    for (const tab of tabs) {
      const btn = document.createElement('button')
      btn.type = 'button'
      btn.className = tab.id === activeId ? 'progress-tab active' : 'progress-tab'
      const label = document.createElement('span')
      label.textContent = tab.mediaName
      btn.appendChild(label)
      const close = document.createElement('span')
      close.className = 'progress-tab-close'
      close.textContent = '×'
      close.addEventListener('click', (e) => {
        e.stopPropagation()
        closeTab(tab.id)
      })
      btn.appendChild(close)
      btn.addEventListener('click', () => activeTabId.set(tab.id))
      tabStrip.appendChild(btn)
    }
  }

  function renderActive(): void {
    const id = activeTabId.get()
    const state = id ? jobStates.get(id) : undefined
    if (!id || !state) {
      empty.hidden = false
      active.hidden = true
      return
    }

    empty.hidden = true
    active.hidden = false
    saveStatusEl.textContent = ''
    saveStatusEl.className = 'save-status'
    fill.style.width = `${Math.min(100, Math.max(0, state.percent))}%`
    stageEl.textContent = stageLabels[state.status] ?? state.status
    messageEl.textContent = state.message

    const terminal = state.status === 'done' || state.status === 'failed' || state.status === 'canceled'
    cancelBtn.hidden = terminal
    saveBtn.disabled = state.status !== 'done'

    if (state.status === 'done') {
      resultEl.textContent = `Subtitle saved to: ${state.srtPath}`
      resultEl.className = 'progress-result success'
    } else if (state.status === 'failed') {
      resultEl.textContent = state.error ?? 'Failed'
      resultEl.className = 'progress-result error'
    } else if (state.status === 'canceled') {
      resultEl.textContent = 'Job canceled'
      resultEl.className = 'progress-result'
    } else {
      resultEl.textContent = ''
      resultEl.className = 'progress-result'
    }
  }

  function closeTab(id: string): void {
    const wasActive = activeTabId.get() === id
    openTabs.set(openTabs.get().filter((t) => t.id !== id))
    if (wasActive) {
      const remaining = openTabs.get()
      activeTabId.set(remaining.length > 0 ? remaining[0].id : null)
    }
  }

  async function openTab(id: string): Promise<void> {
    if (jobStates.has(id)) return
    // Placeholder so a second call (before the fetch below resolves) is a
    // no-op, and so closing the tab mid-fetch can be detected on resume.
    jobStates.set(id, { status: 'queued', percent: 0, message: '', unsubscribe: () => {} })

    const job = await api.job(id)
    const state = jobStates.get(id)
    if (!state) return // tab was closed before this fetch resolved

    state.status = job.status
    state.percent = job.percent
    state.message = job.message ?? ''
    state.srtPath = job.srtPath
    state.error = job.error
    state.unsubscribe = subscribeJobEvents(id, {
      onProgress: (data) => {
        state.status = data.stage
        state.percent = data.percent
        state.message = data.message ?? ''
        if (activeTabId.get() === id) renderActive()
      },
      onDone: (data) => {
        state.status = 'done'
        state.percent = 100
        state.srtPath = data.srtPath
        if (activeTabId.get() === id) renderActive()
        updateSaveAllButton()
        onJobFinished()
      },
      onError: (data) => {
        state.status = 'failed'
        state.error = data.message
        if (activeTabId.get() === id) renderActive()
        onJobFinished()
      },
      onCanceled: () => {
        state.status = 'canceled'
        if (activeTabId.get() === id) renderActive()
        onJobFinished()
      },
    })

    if (activeTabId.get() === id) renderActive()
    updateSaveAllButton()
  }

  openTabs.subscribe((tabs) => {
    const ids = new Set(tabs.map((t) => t.id))
    for (const [id, state] of jobStates) {
      if (!ids.has(id)) {
        state.unsubscribe()
        jobStates.delete(id)
      }
    }
    tabs.forEach((tab) => {
      if (!jobStates.has(tab.id)) openTab(tab.id)
    })
    renderTabs()
    updateSaveAllButton()
  })

  activeTabId.subscribe(() => {
    renderTabs()
    renderActive()
  })

  cancelBtn.addEventListener('click', async () => {
    const id = activeTabId.get()
    if (!id) return
    cancelBtn.disabled = true
    try {
      await api.cancelJob(id)
    } catch (err) {
      resultEl.textContent = err instanceof Error ? err.message : String(err)
      resultEl.className = 'progress-result error'
    } finally {
      cancelBtn.disabled = false
    }
  })

  async function openPicker(mode: 'single' | 'all'): Promise<void> {
    pickerMode = mode
    picker.hidden = false
    saveStatusEl.textContent = ''
    const drives = await api.drives()
    driveSelect.innerHTML = drives
      .map((d) => `<option value="${escapeHtml(d.name)}">${escapeHtml(d.name)} ${escapeHtml(d.label)}</option>`)
      .join('')
    if (drives.length > 0) await navigatePicker(drives[0].name)
  }

  async function navigatePicker(path: string): Promise<void> {
    try {
      const listing = await api.browse(path, 'dir')
      pickerPath = listing.path
      currentPathEl.textContent = listing.path
      pickerUpBtn.hidden = !listing.parent
      pickerUpBtn.onclick = () => navigatePicker(listing.parent!)
      renderPickerEntries(listing.entries)
    } catch (err) {
      pickerEntryList.innerHTML = `<li class="entry-error">${escapeHtml(err instanceof Error ? err.message : String(err))}</li>`
    }
  }

  function renderPickerEntries(entries: Entry[]): void {
    pickerEntryList.innerHTML = ''
    if (entries.length === 0) {
      pickerEntryList.innerHTML = '<li class="entry-empty">(empty)</li>'
      return
    }
    for (const entry of entries) {
      const li = document.createElement('li')
      li.className = `entry entry-${entry.type}`
      li.textContent = `📁 ${entry.name}`
      li.addEventListener('click', () => navigatePicker(entry.path))
      pickerEntryList.appendChild(li)
    }
  }

  function closePicker(): void {
    picker.hidden = true
    pickerMode = null
  }

  driveSelect.addEventListener('change', () => navigatePicker(driveSelect.value))
  cancelPickerBtn.addEventListener('click', closePicker)
  saveBtn.addEventListener('click', () => openPicker('single'))
  saveAllBtn.addEventListener('click', () => openPicker('all'))

  selectDirBtn.addEventListener('click', async () => {
    const destDir = pickerPath
    const mode = pickerMode
    closePicker()
    if (!destDir || !mode) return

    if (mode === 'single') {
      const id = activeTabId.get()
      if (!id) return
      try {
        const res = await api.exportJob(id, destDir)
        saveStatusEl.textContent = `Saved to: ${res.destPath}`
        saveStatusEl.className = 'save-status success'
      } catch (err) {
        saveStatusEl.textContent = err instanceof Error ? err.message : String(err)
        saveStatusEl.className = 'save-status error'
      }
      return
    }

    const doneTabs = openTabs.get().filter((t) => jobStates.get(t.id)?.status === 'done')
    const results = await Promise.allSettled(doneTabs.map((t) => api.exportJob(t.id, destDir)))
    const failures: string[] = []
    let succeeded = 0
    results.forEach((result, i) => {
      if (result.status === 'fulfilled') {
        succeeded++
      } else {
        const message = result.reason instanceof Error ? result.reason.message : String(result.reason)
        failures.push(`${doneTabs[i].mediaName}: ${message}`)
      }
    })
    saveStatusEl.textContent =
      failures.length === 0 ? `Saved ${succeeded}/${doneTabs.length}` : `Saved ${succeeded}/${doneTabs.length} — ${failures.join('; ')}`
    saveStatusEl.className = failures.length === 0 ? 'save-status success' : 'save-status error'
  })
}
