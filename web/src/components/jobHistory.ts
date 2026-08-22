import { api, type Job } from '../api'
import { escapeHtml } from '../dom'
import { activeJobId } from '../state'

const statusLabels: Record<string, string> = {
  queued: '排队中',
  extracting_audio: '提取音频中',
  transcribing: '转录中',
  done: '已完成',
  failed: '失败',
  canceled: '已取消',
}

const inProgressStatuses = new Set(['queued', 'extracting_audio', 'transcribing'])

export interface JobHistoryHandle {
  refresh(): void
}

export function mountJobHistory(root: HTMLElement): JobHistoryHandle {
  root.innerHTML = `
    <div class="panel">
      <h2>任务历史</h2>
      <ul class="job-list"></ul>
    </div>
  `
  const list = root.querySelector<HTMLUListElement>('.job-list')!

  function render(jobs: Job[]): void {
    list.innerHTML = ''
    if (jobs.length === 0) {
      list.innerHTML = '<li class="job-empty">暂无任务</li>'
      return
    }

    const activeId = activeJobId.get()
    for (const j of jobs) {
      const name = j.request.videoPath.split(/[\\/]/).pop() ?? j.request.videoPath
      const statusText = statusLabels[j.status] ?? j.status
      const percentSuffix = inProgressStatuses.has(j.status) ? ` (${Math.round(j.percent)}%)` : ''

      const li = document.createElement('li')
      li.className = `job-item job-${j.status}`
      if (j.id === activeId) li.classList.add('active')
      li.innerHTML = `
        <div class="job-name">${escapeHtml(name)}</div>
        <div class="job-status">${escapeHtml(statusText + percentSuffix)}</div>
      `
      li.addEventListener('click', () => activeJobId.set(j.id))
      list.appendChild(li)
    }
  }

  async function refresh(): Promise<void> {
    const jobs = await api.jobs()
    render(jobs)
  }

  activeJobId.subscribe(() => refresh())

  return { refresh }
}
