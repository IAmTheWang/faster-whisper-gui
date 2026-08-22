import './styles.css'
import { api } from './api'
import { mountDirectoryBrowser } from './components/directoryBrowser'
import { mountJobConfigPanel } from './components/jobConfigPanel'
import { mountJobHistory } from './components/jobHistory'
import { mountProgressPanel } from './components/progressPanel'

const app = document.querySelector<HTMLDivElement>('#app')!
app.innerHTML = `
  <header class="app-header">
    <h1>faster-whisper-gui</h1>
    <div class="health-banner" hidden></div>
  </header>
  <main class="app-layout">
    <section class="browser-column"></section>
    <section class="main-column">
      <div class="config-column"></div>
      <div class="progress-column"></div>
      <div class="history-column"></div>
    </section>
  </main>
`

const healthBanner = app.querySelector<HTMLDivElement>('.health-banner')!
const browserColumn = app.querySelector<HTMLElement>('.browser-column')!
const configColumn = app.querySelector<HTMLElement>('.config-column')!
const progressColumn = app.querySelector<HTMLElement>('.progress-column')!
const historyColumn = app.querySelector<HTMLElement>('.history-column')!

async function checkHealth(): Promise<void> {
  try {
    const health = await api.health()
    const problems: string[] = []
    if (!health.ffmpeg.ok) problems.push(`ffmpeg: ${health.ffmpeg.detail}`)
    if (!health.whisperCli.ok) problems.push(`whisper-cli: ${health.whisperCli.detail}`)

    if (problems.length > 0) {
      healthBanner.hidden = false
      healthBanner.textContent = `环境检测未通过 —— ${problems.join('；')}`
    } else {
      healthBanner.hidden = true
    }
  } catch (err) {
    healthBanner.hidden = false
    healthBanner.textContent = `无法连接后端: ${err instanceof Error ? err.message : String(err)}`
  }
}

const jobHistory = mountJobHistory(historyColumn)
mountDirectoryBrowser(browserColumn)
mountJobConfigPanel(configColumn, () => jobHistory.refresh())
mountProgressPanel(progressColumn, () => jobHistory.refresh())

checkHealth()
