import { api, type Job } from '../api'
import { basenameOf, escapeHtml } from '../dom'
import { activeTabId, openTabs } from '../state'

const statusLabels: Record<string, string> = {
  queued: 'Queued',
  extracting_audio: 'Extracting audio',
  transcribing: 'Transcribing',
  done: 'Done',
  failed: 'Failed',
  canceled: 'Canceled',
}

const inProgressStatuses = new Set(['queued', 'extracting_audio', 'transcribing'])

export interface JobHistoryHandle {
  refresh(): void
}

export function mountJobHistory(root: HTMLElement): JobHistoryHandle {
  root.innerHTML = `
    <div class="panel">
      <h2>Job History</h2>
      <ul class="job-list"></ul>
    </div>
  `
  const list = root.querySelector<HTMLUListElement>('.job-list')!

  function render(jobs: Job[]): void {
    list.innerHTML = ''
    if (jobs.length === 0) {
      list.innerHTML = '<li class="job-empty">No jobs yet</li>'
      return
    }

    const activeId = activeTabId.get()
    for (const j of jobs) {
      const name = basenameOf(j.request.mediaPath)
      const statusText = statusLabels[j.status] ?? j.status
      const percentSuffix = inProgressStatuses.has(j.status) ? ` (${Math.round(j.percent)}%)` : ''

      const li = document.createElement('li')
      li.className = `job-item job-${j.status}`
      if (j.id === activeId) li.classList.add('active')
      li.innerHTML = `
        <div class="job-name">${escapeHtml(name)}</div>
        <div class="job-status">${escapeHtml(statusText + percentSuffix)}</div>
      `
      li.addEventListener('click', () => {
        if (!openTabs.get().some((t) => t.id === j.id)) {
          openTabs.set([...openTabs.get(), { id: j.id, mediaName: name }])
        }
        activeTabId.set(j.id)
      })
      list.appendChild(li)
    }
  }

  async function refresh(): Promise<void> {
    const jobs = await api.jobs()
    render(jobs)
  }

  activeTabId.subscribe(() => refresh())

  return { refresh }
}
