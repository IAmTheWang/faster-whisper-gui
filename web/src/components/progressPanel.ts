import { api, isTerminalStatus, jobStatusLabels, subscribeJobEvents, type JobEventHandlers } from '../api'
import { errMsg, requireEl } from '../dom'
import { activeTabId, openTabs } from '../state'
import { mountDirPicker } from './dirPicker'
import { exportCompleted, summarizeExport } from './progress/exportJobs'
import {
  applyCanceled,
  applyDone,
  applyError,
  applyFailure,
  applyProgress,
  applySnapshot,
  isFinished,
  isNotFound,
  LOST_CONNECTION,
  newTabState,
  UNKNOWN_JOB,
  type SaveStatus,
  type TabState,
} from './progress/tabState'

export interface ProgressPanelHandle {
  // Unsubscribes from the shared stores and every job's event stream.
  dispose(): void
}

// mountProgressPanel keeps one TabState per open Progress tab, independent
// of which tab is frontmost — this is what lets a backgrounded job keep
// progressing (its events keep arriving) while another tab is being viewed.
// State transitions live in progress/tabState.ts, the export orchestration in
// progress/exportJobs.ts; this file is only the DOM and the wiring.
export function mountProgressPanel(
  root: HTMLElement,
  onJobFinished: () => void,
  onJobProgress: () => void = () => {},
): ProgressPanelHandle {
  root.innerHTML = `
    <div class="panel">
      <h2>Progress</h2>
      <div class="progress-tabs" role="tablist" aria-label="Jobs"></div>
      <div class="progress-empty">No job started yet</div>
      <div class="progress-active" role="tabpanel" hidden>
        <div class="progress-bar" role="progressbar" aria-label="Job progress" aria-valuemin="0" aria-valuemax="100" aria-valuenow="0"><div class="progress-bar-fill"></div></div>
        <div class="progress-stage" aria-live="polite"></div>
        <div class="progress-message"></div>
        <button type="button" class="btn-secondary cancel-btn">Cancel</button>
        <div class="progress-result" aria-live="polite"></div>
        <div class="progress-save-actions">
          <button type="button" class="btn-secondary save-btn">Save</button>
          <button type="button" class="btn-secondary save-all-btn">Save All Completed</button>
        </div>
        <div class="save-status" aria-live="polite"></div>
      </div>
      <div class="picker-mount"></div>
    </div>
  `

  const tabStrip = requireEl<HTMLDivElement>(root, '.progress-tabs')
  const empty = requireEl<HTMLDivElement>(root, '.progress-empty')
  const active = requireEl<HTMLDivElement>(root, '.progress-active')
  const panelId = 'progress-panel-active'
  active.id = panelId
  const fill = requireEl<HTMLDivElement>(root, '.progress-bar-fill')
  const progressBar = requireEl<HTMLDivElement>(root, '.progress-bar')
  const stageEl = requireEl<HTMLDivElement>(root, '.progress-stage')
  const messageEl = requireEl<HTMLDivElement>(root, '.progress-message')
  const cancelBtn = requireEl<HTMLButtonElement>(root, '.cancel-btn')
  const resultEl = requireEl<HTMLDivElement>(root, '.progress-result')
  const saveBtn = requireEl<HTMLButtonElement>(root, '.save-btn')
  const saveAllBtn = requireEl<HTMLButtonElement>(root, '.save-all-btn')
  const saveStatusEl = requireEl<HTMLDivElement>(root, '.save-status')

  const picker = mountDirPicker(requireEl<HTMLDivElement>(root, '.picker-mount'))

  const jobStates = new Map<string, TabState>()
  // True while an export is running, so Save buttons can't double-fire.
  let exporting = false

  function updateSaveAllButton(): void {
    saveAllBtn.disabled = exporting || ![...jobStates.values()].some((s) => s.status === 'done')
  }

  // ---- tab strip ----------------------------------------------------------

  // The strip is only rebuilt when something it shows changed.
  let tabsSignature = ''
  // A tab that should take keyboard focus on the next rebuild (arrow keys).
  let pendingFocusId: string | undefined

  function renderTabs(): void {
    const tabs = openTabs.get()
    const activeId = activeTabId.get()
    const signature = `${activeId ?? ''}|${tabs.map((t) => `${t.id}:${t.mediaName}`).join('|')}`
    if (signature === tabsSignature && pendingFocusId === undefined) return
    tabsSignature = signature

    // The strip is rebuilt wholesale; keep keyboard focus on the same tab.
    const focused = document.activeElement as HTMLElement | null
    const focusedTabId =
      pendingFocusId ??
      (focused && focused.classList.contains('progress-tab-label') && tabStrip.contains(focused)
        ? focused.dataset.tabId
        : undefined)
    pendingFocusId = undefined
    let toFocus: HTMLElement | null = null
    let activeLabel: HTMLElement | null = null

    const frag = document.createDocumentFragment()
    // Roving tabindex: only the selected tab is in the Tab order; arrow keys
    // move between tabs. With nothing selected, the first tab stays reachable.
    const tabbableId = tabs.some((t) => t.id === activeId) ? activeId : tabs[0]?.id
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
      label.tabIndex = tab.id === tabbableId ? 0 : -1
      label.dataset.tabId = tab.id
      label.id = `progress-tab-${tab.id}`
      label.setAttribute('aria-controls', panelId)
      label.textContent = tab.mediaName
      label.title = `${tab.mediaName} (press Delete to close)`
      wrap.appendChild(label)
      if (isActive) activeLabel = label

      // The × is mouse-only (aria-hidden, not focusable) so the tablist only
      // contains tabs; keyboard users close a tab with Delete instead.
      const close = document.createElement('button')
      close.type = 'button'
      close.className = 'progress-tab-close'
      close.dataset.tabId = tab.id
      close.setAttribute('aria-hidden', 'true')
      close.tabIndex = -1
      close.title = `Close ${tab.mediaName}`
      close.textContent = '×'
      wrap.appendChild(close)

      frag.appendChild(wrap)
      if (tab.id === focusedTabId) toFocus = label
    }
    tabStrip.replaceChildren(frag)
    // If the focused tab was just closed, move focus to the now-active tab
    // instead of letting it fall back to <body>.
    const focusLost = focusedTabId !== undefined && !toFocus
    ;(toFocus ?? (focusLost ? activeLabel : null))?.focus()
    if (activeLabel) active.setAttribute('aria-labelledby', (activeLabel as HTMLElement).id)
    else active.removeAttribute('aria-labelledby')
  }

  // One set of listeners for the whole strip (it is rebuilt often).
  tabStrip.addEventListener('click', (e) => {
    const target = e.target as HTMLElement
    const close = target.closest<HTMLElement>('.progress-tab-close')
    if (close?.dataset.tabId) return closeTab(close.dataset.tabId)
    const id = target.closest<HTMLElement>('.progress-tab-label')?.dataset.tabId
    if (id) activeTabId.set(id)
  })
  tabStrip.addEventListener('keydown', (e) => {
    const label = (e.target as HTMLElement).closest<HTMLElement>('.progress-tab-label')
    const id = label?.dataset.tabId
    if (!id) return
    if (e.key === 'Delete') {
      closeTab(id)
      return
    }
    const tabs = openTabs.get()
    const idx = tabs.findIndex((t) => t.id === id)
    let next: number
    switch (e.key) {
      case 'ArrowRight':
        next = (idx + 1) % tabs.length
        break
      case 'ArrowLeft':
        next = (idx - 1 + tabs.length) % tabs.length
        break
      case 'Home':
        next = 0
        break
      case 'End':
        next = tabs.length - 1
        break
      default:
        return
    }
    const nextId = tabs[next]?.id
    if (!nextId) return
    e.preventDefault()
    pendingFocusId = nextId
    activeTabId.set(nextId)
  })

  // ---- active tab ---------------------------------------------------------

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

    cancelBtn.hidden = isTerminalStatus(state.status)
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

  function repaint(id: string): void {
    if (activeTabId.get() === id) renderActive()
    updateSaveAllButton()
  }

  function closeTab(id: string): void {
    const wasActive = activeTabId.get() === id
    openTabs.set(openTabs.get().filter((t) => t.id !== id))
    if (wasActive) {
      const remaining = openTabs.get()
      activeTabId.set(remaining[0]?.id ?? null)
    }
  }

  // ---- job lifecycle ------------------------------------------------------

  // sync fetches the job's current state and applies it — on opening a tab, and
  // whenever the event stream (re)connects, since events sent while it was down
  // are gone. A snapshot older than a live event that arrived meanwhile is
  // dropped (tabState's `events` counter), so it can never roll progress back.
  async function sync(id: string, state: TabState): Promise<void> {
    const eventsBefore = state.events
    // Only a job seen unfinished that is now finished counts as "just finished";
    // opening a tab for an old job must not trigger a history refresh.
    const justFinishedCandidate = state.synced && !isFinished(state)
    try {
      const job = await api.job(id)
      // Identity, not just presence: the tab may have been closed and reopened
      // while the fetch was in flight, and the reopened tab owns its own state.
      if (jobStates.get(id) !== state || state.events !== eventsBefore) return
      applySnapshot(state, job)
    } catch (err) {
      if (jobStates.get(id) !== state) return
      if (!state.synced) {
        // Show why the tab is empty; closing and reopening the tab retries.
        applyFailure(state, `Could not load job: ${errMsg(err)}`)
        state.unsubscribe()
      } else if (isNotFound(err)) {
        applyFailure(state, UNKNOWN_JOB)
        state.unsubscribe()
      } else {
        console.warn('could not refresh job', id, err)
      }
    }
    if (isFinished(state)) {
      state.unsubscribe() // nothing more will come for a finished job
      if (justFinishedCandidate) onJobFinished()
    }
    repaint(id)
  }

  function openTab(id: string): void {
    if (jobStates.has(id)) return
    const state = newTabState()
    jobStates.set(id, state)

    const finish = (): void => {
      repaint(id)
      onJobFinished()
    }
    const handlers: JobEventHandlers = {
      onProgress: (data) => {
        applyProgress(state, data)
        repaint(id)
        onJobProgress()
      },
      onDone: (data) => {
        applyDone(state, data)
        finish()
      },
      onError: (data) => {
        applyError(state, data)
        finish()
      },
      onCanceled: () => {
        applyCanceled(state)
        finish()
      },
      // Fires when the shared stream opens or reconnects: also covers the very
      // first load of this tab, so there is no separate initial fetch.
      onSync: () => void sync(id, state),
      onDisconnect: () => {
        // The stream is gone for good: resync once, and if the job is still
        // unfinished with nothing left to tell us, say so instead of hanging.
        void sync(id, state).then(() => {
          if (jobStates.get(id) !== state || isFinished(state)) return
          applyFailure(state, LOST_CONNECTION)
          finish()
        })
      },
    }
    state.unsubscribe = subscribeJobEvents(id, handlers)
    repaint(id)
  }

  const unsubscribeTabs = openTabs.subscribe((tabs) => {
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

  const unsubscribeActive = activeTabId.subscribe(() => {
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

  // ---- saving a copy elsewhere --------------------------------------------

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

  // The summary is shown on statusTargetId (the tab that was frontmost when the
  // picker was opened).
  async function exportAll(statusTargetId: string, destDir: string): Promise<void> {
    setExporting(true)
    try {
      const outcome = await exportCompleted(
        (id, dir) => api.exportJob(id, dir),
        openTabs.get(),
        (id) => jobStates.get(id),
        destDir,
      )
      setSaveStatus(statusTargetId, summarizeExport(outcome))
    } finally {
      setExporting(false)
    }
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

  return {
    dispose() {
      unsubscribeTabs()
      unsubscribeActive()
      for (const state of jobStates.values()) state.unsubscribe()
      jobStates.clear()
    },
  }
}
