import { api, isTerminalStatus, jobStatusLabels, type Job } from '../api'
import { basenameOf, delegateActivate, errMsg, escapeHtml, requireEl, rowButton } from '../dom'
import { activeTabId, openTabs } from '../state'

// Minimum gap between history refreshes triggered by progress events.
const PROGRESS_REFRESH_MS = 2000

// historySignature is a cheap fingerprint of everything render() shows, so a
// refresh that changed nothing (the common case while a job runs) can skip
// rebuilding the list.
export function historySignature(jobs: readonly Job[], activeId: string | null): string {
  return `${activeId ?? ''}|${jobs
    .map((j) => `${j.id}:${j.status}:${isTerminalStatus(j.status) ? '' : Math.round(j.percent)}:${j.request.mediaPath}`)
    .join('|')}`
}

export interface JobHistoryHandle {
  // Stops the pending throttled refresh and the activeTabId subscription.
  dispose(): void
  refresh(): void
  // Like refresh, but coalesced to at most about one request per
  // PROGRESS_REFRESH_MS — for high-frequency callers (progress events).
  refreshThrottled(): void
}

export function mountJobHistory(root: HTMLElement): JobHistoryHandle {
  root.innerHTML = `
    <div class="panel">
      <h2>Job History</h2>
      <div class="form-error history-error" role="alert"></div>
      <ul class="job-list"></ul>
    </div>
  `
  const list = requireEl<HTMLUListElement>(root, '.job-list')
  const errorEl = requireEl<HTMLDivElement>(root, '.history-error')

  let lastSignature = ''
  // The jobs currently shown, by id, for the delegated click handler.
  let jobsById = new Map<string, Job>()

  function render(jobs: Job[]): void {
    const signature = historySignature(jobs, activeTabId.get())
    if (signature === lastSignature) return
    lastSignature = signature
    jobsById = new Map(jobs.map((j) => [j.id, j]))

    // The list is rebuilt wholesale; keep keyboard focus on the same row.
    const focusedId = list.contains(document.activeElement)
      ? (document.activeElement as HTMLElement).closest<HTMLElement>('.job-item')?.dataset.id
      : undefined

    list.innerHTML = ''
    if (jobs.length === 0) {
      list.innerHTML = '<li class="job-empty">No jobs yet</li>'
      return
    }

    const activeId = activeTabId.get()
    for (const j of jobs) {
      const name = basenameOf(j.request.mediaPath)
      const statusText = jobStatusLabels[j.status] ?? j.status
      const percentSuffix = isTerminalStatus(j.status) ? '' : ` (${Math.round(j.percent)}%)`

      const li = document.createElement('li')
      li.className = `job-item job-${j.status}`
      li.dataset.id = j.id
      if (j.id === activeId) li.classList.add('active')
      const btn = rowButton()
      btn.innerHTML = `
        <span class="job-name">${escapeHtml(name)}</span>
        <span class="job-status">${escapeHtml(statusText + percentSuffix)}</span>
      `
      li.appendChild(btn)
      list.appendChild(li)
      if (j.id === focusedId) btn.focus()
    }
  }

  // Monotonic counter: a slower, earlier response never overwrites a newer one.
  let refreshSeq = 0

  async function refreshNow(): Promise<void> {
    const mySeq = ++refreshSeq
    try {
      const jobs = await api.jobs()
      if (mySeq !== refreshSeq) return
      errorEl.textContent = ''
      render(jobs)
    } catch (err) {
      if (mySeq !== refreshSeq) return
      errorEl.textContent = `Could not load job history: ${errMsg(err)}`
    }
  }

  let lastThrottledRun = 0
  let throttleTimer: ReturnType<typeof setTimeout> | null = null

  function runThrottled(): void {
    throttleTimer = null
    lastThrottledRun = Date.now()
    void refreshNow()
  }

  function refreshThrottled(): void {
    if (throttleTimer !== null) return // a trailing refresh is already scheduled
    const wait = lastThrottledRun + PROGRESS_REFRESH_MS - Date.now()
    if (wait <= 0) {
      runThrottled()
    } else {
      throttleTimer = setTimeout(runThrottled, wait)
    }
  }

  // Clicking a row opens (or refocuses, if already open) that job's Progress tab.
  delegateActivate(list, '.job-item', (li) => {
    const job = li.dataset.id ? jobsById.get(li.dataset.id) : undefined
    if (!job) return
    if (!openTabs.get().some((t) => t.id === job.id)) {
      openTabs.set([...openTabs.get(), { id: job.id, mediaName: basenameOf(job.request.mediaPath) }])
    }
    activeTabId.set(job.id)
  })

  const unsubscribeActive = activeTabId.subscribe(() => void refreshNow())

  return {
    refresh: () => void refreshNow(),
    refreshThrottled,
    dispose() {
      unsubscribeActive()
      if (throttleTimer !== null) clearTimeout(throttleTimer)
      throttleTimer = null
      refreshSeq++ // ignore a response still in flight
    },
  }
}
