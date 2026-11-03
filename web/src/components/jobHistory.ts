import { api, isTerminalStatus, jobStatusLabels, type Job } from '../api'
import { basenameOf, errMsg, escapeHtml, makeActivatable } from '../dom'
import { activeTabId, openTabs } from '../state'

// Minimum gap between history refreshes triggered by progress events.
const PROGRESS_REFRESH_MS = 2000

export interface JobHistoryHandle {
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
  const list = root.querySelector<HTMLUListElement>('.job-list')!
  const errorEl = root.querySelector<HTMLDivElement>('.history-error')!

  function render(jobs: Job[]): void {
    // The list is rebuilt wholesale; keep keyboard focus on the same row.
    const focusedId = list.contains(document.activeElement)
      ? (document.activeElement as HTMLElement).dataset.id
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
      li.innerHTML = `
        <div class="job-name">${escapeHtml(name)}</div>
        <div class="job-status">${escapeHtml(statusText + percentSuffix)}</div>
      `
      makeActivatable(li, () => {
        if (!openTabs.get().some((t) => t.id === j.id)) {
          openTabs.set([...openTabs.get(), { id: j.id, mediaName: name }])
        }
        activeTabId.set(j.id)
      })
      list.appendChild(li)
      if (j.id === focusedId) li.focus()
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

  activeTabId.subscribe(() => void refreshNow())

  return { refresh: () => void refreshNow(), refreshThrottled }
}
