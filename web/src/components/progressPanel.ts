import { api, isTerminalStatus, jobStatusLabels, subscribeJobEvents, type JobStatus } from '../api'
import { basenameOf, errMsg } from '../dom'
import { activeTabId, openTabs } from '../state'
import { mountDirPicker } from './dirPicker'

interface SaveStatus {
  text: string
  kind: 'success' | 'error'
}

interface TabState {
  status: JobStatus
  percent: number
  message: string
  srtPath?: string
  error?: string
  // Kept per tab (not in the DOM) so renderActive() can redraw on every
  // progress event or tab switch without losing them.
  saveStatus?: SaveStatus
  cancelError?: string
  unsubscribe: () => void
}

// mountProgressPanel keeps one TabState per open Progress tab, independent
// of which tab is frontmost — this is what lets a backgrounded job keep
// progressing (SSE stays connected) while another tab is being viewed.
export function mountProgressPanel(
  root: HTMLElement,
  onJobFinished: () => void,
  onJobProgress: () => void = () => {},
): void {
  root.innerHTML = `
    <div class="panel">
      <h2>Progress</h2>
      <div class="progress-tabs" role="tablist" aria-label="Jobs"></div>
      <div class="progress-empty">No job started yet</div>
      <div class="progress-active" role="tabpanel" hidden>
        <div class="progress-bar" role="progressbar" aria-label="Job progress" aria-valuemin="0" aria-valuemax="100" aria-valuenow="0"><div class="progress-bar-fill"></div></div>
        <div class="progress-stage"></div>
        <div class="progress-message"></div>
        <button type="button" class="btn-secondary cancel-btn">Cancel</button>
        <div class="progress-result"></div>
        <div class="progress-save-actions">
          <button type="button" class="btn-secondary save-btn">Save</button>
          <button type="button" class="btn-secondary save-all-btn">Save All Completed</button>
        </div>
        <div class="save-status" aria-live="polite"></div>
      </div>
      <div class="picker-mount"></div>
    </div>
  `

  const tabStrip = root.querySelector<HTMLDivElement>('.progress-tabs')!
  const empty = root.querySelector<HTMLDivElement>('.progress-empty')!
  const active = root.querySelector<HTMLDivElement>('.progress-active')!
  const panelId = 'progress-panel-active'
  active.id = panelId
  const fill = root.querySelector<HTMLDivElement>('.progress-bar-fill')!
  const stageEl = root.querySelector<HTMLDivElement>('.progress-stage')!
  const messageEl = root.querySelector<HTMLDivElement>('.progress-message')!
  const cancelBtn = root.querySelector<HTMLButtonElement>('.cancel-btn')!
  const resultEl = root.querySelector<HTMLDivElement>('.progress-result')!
  const saveBtn = root.querySelector<HTMLButtonElement>('.save-btn')!
  const saveAllBtn = root.querySelector<HTMLButtonElement>('.save-all-btn')!
  const saveStatusEl = root.querySelector<HTMLDivElement>('.save-status')!

  const picker = mountDirPicker(root.querySelector<HTMLDivElement>('.picker-mount')!)
  const progressBar = root.querySelector<HTMLDivElement>('.progress-bar')!

  const jobStates = new Map<string, TabState>()
  // True while an export is running, so Save buttons can't double-fire.
  let exporting = false

  function updateSaveAllButton(): void {
    saveAllBtn.disabled = exporting || ![...jobStates.values()].some((s) => s.status === 'done')
  }

  function renderTabs(): void {
    const tabs = openTabs.get()
    const activeId = activeTabId.get()
    // The strip is rebuilt wholesale; keep keyboard focus on the same tab.
    const focused = document.activeElement as HTMLElement | null
    const focusedTabId =
      focused && focused.classList.contains('progress-tab-label') && tabStrip.contains(focused)
        ? focused.dataset.tabId
        : undefined
    let toFocus: HTMLElement | null = null
    let activeLabel: HTMLElement | null = null

    tabStrip.innerHTML = ''
    for (const tab of tabs) {
      const isActive = tab.id === activeId
      const wrap = document.createElement('div')
      wrap.className = isActive ? 'progress-tab active' : 'progress-tab'
      wrap.setAttribute('role', 'presentation')

      const label = document.createElement('button')
      label.type = 'button'
      label.className = 'progress-tab-label'
      label.setAttribute('role', 'tab')
      label.setAttribute('aria-selected', String(isActive))
      label.dataset.tabId = tab.id
      label.id = `progress-tab-${tab.id}`
      label.setAttribute('aria-controls', panelId)
      label.textContent = tab.mediaName
      label.title = `${tab.mediaName} (press Delete to close)`
      label.addEventListener('click', () => activeTabId.set(tab.id))
      // The × is mouse-only (aria-hidden, not focusable) so the tablist only
      // contains tabs; keyboard users close a tab with Delete instead.
      label.addEventListener('keydown', (e) => {
        if (e.key === 'Delete') closeTab(tab.id)
      })
      wrap.appendChild(label)
      if (isActive) activeLabel = label

      const close = document.createElement('button')
      close.type = 'button'
      close.className = 'progress-tab-close'
      close.setAttribute('aria-hidden', 'true')
      close.tabIndex = -1
      close.title = `Close ${tab.mediaName}`
      close.textContent = '×'
      close.addEventListener('click', () => closeTab(tab.id))
      wrap.appendChild(close)

      tabStrip.appendChild(wrap)
      if (tab.id === focusedTabId) toFocus = label
    }
    // If the focused tab was just closed, move focus to the now-active tab
    // instead of letting it fall back to <body>.
    const focusLost = focusedTabId !== undefined && !toFocus
    ;(toFocus ?? (focusLost ? activeLabel : null))?.focus()
    if (activeLabel) active.setAttribute('aria-labelledby', (activeLabel as HTMLElement).id)
    else active.removeAttribute('aria-labelledby')
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
    const pct = Math.min(100, Math.max(0, state.percent))
    fill.style.width = `${pct}%`
    progressBar.setAttribute('aria-valuenow', String(Math.round(pct)))
    stageEl.textContent = jobStatusLabels[state.status] ?? state.status
    messageEl.textContent = state.message

    const terminal = isTerminalStatus(state.status)
    cancelBtn.hidden = terminal
    saveBtn.disabled = exporting || state.status !== 'done'
    saveStatusEl.textContent = state.saveStatus?.text ?? ''
    saveStatusEl.className = state.saveStatus ? `save-status ${state.saveStatus.kind}` : 'save-status'

    if (state.status === 'done') {
      resultEl.textContent = `Subtitle saved to: ${state.srtPath}`
      resultEl.className = 'progress-result success'
    } else if (state.status === 'failed') {
      resultEl.textContent = state.error ?? 'Failed'
      resultEl.className = 'progress-result error'
    } else if (state.status === 'canceled') {
      resultEl.textContent = 'Job canceled'
      resultEl.className = 'progress-result'
    } else if (state.cancelError) {
      resultEl.textContent = state.cancelError
      resultEl.className = 'progress-result error'
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

    const placeholder = jobStates.get(id)!
    let job: Awaited<ReturnType<typeof api.job>>
    try {
      job = await api.job(id)
    } catch (err) {
      // Show why the tab is empty; closing and reopening the tab retries.
      if (jobStates.get(id) === placeholder) {
        placeholder.status = 'failed'
        placeholder.error = `Could not load job: ${errMsg(err)}`
        if (activeTabId.get() === id) renderActive()
      }
      return
    }
    // Identity, not just presence: the tab may have been closed and reopened
    // while the fetch was in flight, and the reopened tab owns its own fetch.
    const state = jobStates.get(id)
    if (!state || state !== placeholder) return

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
        onJobProgress()
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
      onDisconnect: () => {
        // Resync from the server; if the job is gone (server restarted) or
        // still unfinished with no stream, say so instead of hanging.
        api
          .job(id)
          .then((j) => {
            state.status = j.status
            state.percent = j.percent
            state.srtPath = j.srtPath
            state.error = j.error
            if (!isTerminalStatus(j.status)) {
              state.status = 'failed'
              state.error = 'Lost connection to the server while the job was running.'
            }
          })
          .catch(() => {
            state.status = 'failed'
            state.error = 'The server no longer knows this job (it may have been restarted).'
          })
          .finally(() => {
            if (activeTabId.get() === id) renderActive()
            updateSaveAllButton()
            onJobFinished()
          })
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
    const state = jobStates.get(id)
    if (state) state.cancelError = undefined
    cancelBtn.disabled = true
    try {
      await api.cancelJob(id)
    } catch (err) {
      if (state) state.cancelError = errMsg(err)
      if (activeTabId.get() === id) renderActive()
    } finally {
      cancelBtn.disabled = false
    }
  })

  function setSaveStatus(id: string, status: SaveStatus): void {
    const state = jobStates.get(id)
    if (state) state.saveStatus = status
    if (activeTabId.get() === id) renderActive()
  }

  function setExporting(value: boolean): void {
    exporting = value
    updateSaveAllButton()
    const id = activeTabId.get()
    const state = id ? jobStates.get(id) : undefined
    saveBtn.disabled = exporting || state?.status !== 'done'
  }

  async function exportOne(jobId: string, destDir: string): Promise<void> {
    setExporting(true)
    try {
      const res = await api.exportJob(jobId, destDir)
      setSaveStatus(jobId, { text: `Saved to: ${res.destPath}`, kind: 'success' })
    } catch (err) {
      setSaveStatus(jobId, { text: errMsg(err), kind: 'error' })
    } finally {
      setExporting(false)
    }
  }

  // Exports every completed tab one at a time. Two jobs whose .srt files share
  // a basename would overwrite each other in one folder, so the later one is
  // skipped and reported instead. The summary is shown on statusTargetId (the
  // tab that was frontmost when the picker was opened).
  async function exportAll(statusTargetId: string, destDir: string): Promise<void> {
    const doneTabs = openTabs.get().filter((t) => jobStates.get(t.id)?.status === 'done')
    setExporting(true)
    const problems: string[] = []
    const claimed = new Map<string, string>() // lower-cased basename -> media name that took it
    let succeeded = 0
    try {
      for (const tab of doneTabs) {
        const srtPath = jobStates.get(tab.id)?.srtPath
        const base = srtPath ? basenameOf(srtPath).toLowerCase() : ''
        const owner = base ? claimed.get(base) : undefined
        if (owner !== undefined) {
          problems.push(`${tab.mediaName}: skipped, "${basenameOf(srtPath!)}" would overwrite the copy from ${owner}`)
          continue
        }
        try {
          await api.exportJob(tab.id, destDir)
          if (base) claimed.set(base, tab.mediaName)
          succeeded++
        } catch (err) {
          problems.push(`${tab.mediaName}: ${errMsg(err)}`)
        }
      }
    } finally {
      setExporting(false)
    }
    setSaveStatus(statusTargetId, {
      text:
        problems.length === 0
          ? `Saved ${succeeded}/${doneTabs.length}`
          : `Saved ${succeeded}/${doneTabs.length} — ${problems.join('; ')}`,
      kind: problems.length === 0 ? 'success' : 'error',
    })
  }

  // The job (or status-target tab) is captured here, when the picker is
  // opened — not read from activeTabId at confirm time, when the user may
  // have switched tabs.
  function openSavePicker(mode: 'single' | 'all'): void {
    const targetId = activeTabId.get()
    if (!targetId) return
    const state = jobStates.get(targetId)
    if (state) state.saveStatus = undefined
    renderActive()
    void picker.open({
      kind: 'dir',
      onSelect: (destDir) => {
        if (mode === 'single') void exportOne(targetId, destDir)
        else void exportAll(targetId, destDir)
      },
    })
  }

  saveBtn.addEventListener('click', () => openSavePicker('single'))
  saveAllBtn.addEventListener('click', () => openSavePicker('all'))
}
