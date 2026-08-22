import { api, subscribeJobEvents } from '../api'
import { activeJobId } from '../state'

const stageLabels: Record<string, string> = {
  queued: '排队中',
  extracting_audio: '正在提取音频',
  transcribing: '正在转录',
}

export function mountProgressPanel(root: HTMLElement, onJobFinished: () => void): void {
  root.innerHTML = `
    <div class="panel">
      <h2>转录进度</h2>
      <div class="progress-empty">尚未开始任务</div>
      <div class="progress-active" hidden>
        <div class="progress-bar"><div class="progress-bar-fill"></div></div>
        <div class="progress-stage"></div>
        <div class="progress-message"></div>
        <button type="button" class="btn-secondary cancel-btn">取消</button>
        <div class="progress-result"></div>
      </div>
    </div>
  `

  const empty = root.querySelector<HTMLDivElement>('.progress-empty')!
  const active = root.querySelector<HTMLDivElement>('.progress-active')!
  const fill = root.querySelector<HTMLDivElement>('.progress-bar-fill')!
  const stageEl = root.querySelector<HTMLDivElement>('.progress-stage')!
  const messageEl = root.querySelector<HTMLDivElement>('.progress-message')!
  const cancelBtn = root.querySelector<HTMLButtonElement>('.cancel-btn')!
  const resultEl = root.querySelector<HTMLDivElement>('.progress-result')!

  let unsubscribeSSE: (() => void) | null = null
  let currentJobId: string | null = null

  function finish(stageText: string, resultText: string, resultClass: string): void {
    stageEl.textContent = stageText
    cancelBtn.hidden = true
    resultEl.textContent = resultText
    resultEl.className = `progress-result ${resultClass}`
    onJobFinished()
  }

  activeJobId.subscribe((jobId) => {
    unsubscribeSSE?.()
    unsubscribeSSE = null
    currentJobId = jobId

    if (!jobId) {
      empty.hidden = false
      active.hidden = true
      return
    }

    empty.hidden = true
    active.hidden = false
    fill.style.width = '0%'
    stageEl.textContent = stageLabels.queued
    messageEl.textContent = ''
    resultEl.textContent = ''
    resultEl.className = 'progress-result'
    cancelBtn.hidden = false

    unsubscribeSSE = subscribeJobEvents(jobId, {
      onProgress: (data) => {
        fill.style.width = `${Math.min(100, Math.max(0, data.percent))}%`
        stageEl.textContent = stageLabels[data.stage] ?? data.stage
        messageEl.textContent = data.message ?? ''
      },
      onDone: (data) => {
        fill.style.width = '100%'
        finish('完成', `字幕已保存到: ${data.srtPath}`, 'success')
      },
      onError: (data) => {
        finish('失败', data.message, 'error')
      },
      onCanceled: () => {
        finish('已取消', '任务已取消', '')
      },
    })
  })

  cancelBtn.addEventListener('click', async () => {
    if (!currentJobId) return
    cancelBtn.disabled = true
    try {
      await api.cancelJob(currentJobId)
    } catch (err) {
      resultEl.textContent = err instanceof Error ? err.message : String(err)
      resultEl.className = 'progress-result error'
    } finally {
      cancelBtn.disabled = false
    }
  })
}
