import { api, type Language, type Model, type OutputMode } from '../api'
import { activeTabId, openTabs, selectedVideos, type TabInfo } from '../state'

export interface JobConfigPanelHandle {
  refreshModels: () => Promise<void>
}

export function mountJobConfigPanel(root: HTMLElement, onJobCreated: () => void): JobConfigPanelHandle {
  root.innerHTML = `
    <div class="panel">
      <h2>Transcription Settings</h2>
      <div class="field">
        <label>Video Files</label>
        <div class="selected-video">No videos selected</div>
      </div>
      <div class="field">
        <label for="model-select">Model</label>
        <select id="model-select" class="model-select"></select>
      </div>
      <div class="field">
        <label for="language-select">Language</label>
        <select id="language-select" class="language-select"></select>
      </div>
      <div class="field">
        <label>Output Location</label>
        <label class="radio-label">
          <input type="radio" name="outputMode" value="same_as_video" checked /> Default (same directory as video)
        </label>
        <label class="radio-label">
          <input type="radio" name="outputMode" value="custom" /> Custom directory
        </label>
        <input type="text" class="output-dir" placeholder="E:\\subtitles" disabled />
      </div>
      <details class="advanced">
        <summary>Advanced Options</summary>
        <div class="field">
          <label for="max-len">Max characters per subtitle line (leave blank for default)</label>
          <input type="number" id="max-len" class="max-len" min="1" />
        </div>
      </details>
      <button type="button" class="btn-primary start-btn">Start Transcription</button>
      <div class="form-error"></div>
    </div>
  `

  const selectedVideoEl = root.querySelector<HTMLDivElement>('.selected-video')!
  const modelSelect = root.querySelector<HTMLSelectElement>('.model-select')!
  const languageSelect = root.querySelector<HTMLSelectElement>('.language-select')!
  const outputModeRadios = root.querySelectorAll<HTMLInputElement>('input[name="outputMode"]')
  const outputDirInput = root.querySelector<HTMLInputElement>('.output-dir')!
  const maxLenInput = root.querySelector<HTMLInputElement>('.max-len')!
  const startBtn = root.querySelector<HTMLButtonElement>('.start-btn')!
  const formError = root.querySelector<HTMLDivElement>('.form-error')!

  function getOutputMode(): OutputMode {
    const checked = root.querySelector<HTMLInputElement>('input[name="outputMode"]:checked')
    return (checked?.value as OutputMode) ?? 'same_as_video'
  }

  outputModeRadios.forEach((radio) => {
    radio.addEventListener('change', () => {
      outputDirInput.disabled = getOutputMode() !== 'custom'
    })
  })

  selectedVideos.subscribe((videos) => {
    if (videos.length === 0) {
      selectedVideoEl.textContent = 'No videos selected'
    } else if (videos.length > 5) {
      selectedVideoEl.textContent = `${videos.length} videos selected`
    } else {
      selectedVideoEl.textContent = videos.map((v) => v.name).join(', ')
    }
  })

  async function loadModels(): Promise<void> {
    const models = await api.models()
    modelSelect.innerHTML = models.map((m: Model) => `<option value="${m.id}">${m.label}</option>`).join('')
    formError.textContent =
      models.length === 0 ? 'No model files detected — put a ggml-*.bin file in the models directory and refresh' : ''
  }

  async function loadLanguages(): Promise<void> {
    const languages = await api.languages()
    languageSelect.innerHTML = languages
      .map((l: Language) => `<option value="${l.code}">${l.label}</option>`)
      .join('')
  }

  startBtn.addEventListener('click', async () => {
    formError.textContent = ''

    const videos = selectedVideos.get()
    if (videos.length === 0) {
      formError.textContent = 'Select at least one video file on the left first'
      return
    }
    if (!modelSelect.value) {
      formError.textContent = 'Select a model first'
      return
    }

    const outputMode = getOutputMode()
    const maxLenValue = maxLenInput.value.trim()
    const language = languageSelect.value || 'auto'
    const outputDir = outputMode === 'custom' ? outputDirInput.value.trim() : undefined
    const maxLen = maxLenValue ? Number(maxLenValue) : undefined

    startBtn.disabled = true
    try {
      const results = await Promise.allSettled(
        videos.map((video) =>
          api.createJob({
            videoPath: video.path,
            modelId: modelSelect.value,
            language,
            outputMode,
            outputDir,
            maxLen,
          }),
        ),
      )

      const newTabs: TabInfo[] = []
      const errors: string[] = []
      results.forEach((result, i) => {
        if (result.status === 'fulfilled') {
          newTabs.push({ id: result.value.jobId, videoName: videos[i].name })
        } else {
          const message = result.reason instanceof Error ? result.reason.message : String(result.reason)
          errors.push(`${videos[i].name}: ${message}`)
        }
      })

      if (newTabs.length > 0) {
        openTabs.set([...openTabs.get(), ...newTabs])
        activeTabId.set(newTabs[0].id)
        onJobCreated()
      }
      formError.textContent = errors.join('; ')
      selectedVideos.set([])
    } finally {
      startBtn.disabled = false
    }
  })

  loadModels()
  loadLanguages()

  return { refreshModels: loadModels }
}
