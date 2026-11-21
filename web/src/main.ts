import './styles.css'
import { api } from './api'
import { errMsg, requireEl } from './dom'
import { mountDirectoryBrowser } from './components/directoryBrowser'
import { mountJobConfigPanel } from './components/jobConfigPanel'
import { mountJobHistory } from './components/jobHistory'
import { mountProgressPanel } from './components/progressPanel'
import { mountSettingsPanel } from './components/settingsPanel'

const app = requireEl<HTMLDivElement>(document, '#app')
app.innerHTML = `
  <header class="app-header">
    <h1>faster-whisper-gui</h1>
    <div class="health-banner" hidden></div>
  </header>
  <main class="app-layout">
    <section class="browser-column"></section>
    <section class="main-column">
      <div class="settings-column"></div>
      <div class="config-column"></div>
      <div class="progress-column"></div>
      <div class="history-column"></div>
    </section>
  </main>
`

const healthBanner = requireEl<HTMLDivElement>(app, '.health-banner')
const browserColumn = requireEl<HTMLElement>(app, '.browser-column')
const settingsColumn = requireEl<HTMLElement>(app, '.settings-column')
const configColumn = requireEl<HTMLElement>(app, '.config-column')
const progressColumn = requireEl<HTMLElement>(app, '.progress-column')
const historyColumn = requireEl<HTMLElement>(app, '.history-column')

async function checkHealth(): Promise<void> {
  try {
    const health = await api.health()
    const problems: string[] = []
    if (!health.ffmpeg.ok) problems.push(`ffmpeg: ${health.ffmpeg.detail}`)
    if (!health.whisperCli.ok) problems.push(`whisper-cli: ${health.whisperCli.detail}`)

    if (problems.length > 0) {
      healthBanner.hidden = false
      healthBanner.textContent = `Environment check failed — ${problems.join('; ')}`
    } else {
      healthBanner.hidden = true
    }
  } catch (err) {
    healthBanner.hidden = false
    healthBanner.textContent = `Cannot reach backend: ${errMsg(err)}`
  }
}

const jobHistory = mountJobHistory(historyColumn)
mountDirectoryBrowser(browserColumn)
const jobConfigPanel = mountJobConfigPanel(configColumn, () => jobHistory.refresh())
const progressPanel = mountProgressPanel(
  progressColumn,
  () => jobHistory.refresh(),
  () => jobHistory.refreshThrottled(),
)
mountSettingsPanel(settingsColumn, () => {
  checkHealth()
  // loadModels reports its own failures inline; this just guards the call.
  jobConfigPanel.refreshModels().catch((err) => console.error('refreshModels failed', err))
})

checkHealth()

// Dev-server hot reload re-runs this module; tear the old panels down first so
// their store subscriptions and the shared event stream don't pile up.
import.meta.hot?.dispose(() => {
  progressPanel.dispose()
  jobHistory.dispose()
})
