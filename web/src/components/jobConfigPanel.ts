import { api, type Language, type Model, type OutputMode } from '../api'
import { activeTabId, openTabs, selectedMedia, type TabInfo } from '../state'

export interface JobConfigPanelHandle {
  refreshModels: () => Promise<void>
}

export function mountJobConfigPanel(root: HTMLElement, onJobCreated: () => void): JobConfigPanelHandle {
  root.innerHTML = `
    <div class="panel">
      <h2>Transcription Settings</h2>
      <div class="field">
        <label>Media Files</label>
        <div class="selected-media">No files selected</div>
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
          <input type="radio" name="outputMode" value="same_as_source" checked /> Default (same directory as the source file)
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

  const selectedMediaEl = root.querySelector<HTMLDivElement>('.selected-media')!
  const modelSelect = root.querySelector<HTMLSelectElement>('.model-select')!
  const languageSelect = root.querySelector<HTMLSelectElement>('.language-select')!
  const outputModeRadios = root.querySelectorAll<HTMLInputElement>('input[name="outputMode"]')
  const outputDirInput = root.querySelector<HTMLInputElement>('.output-dir')!
  const maxLenInput = root.querySelector<HTMLInputElement>('.max-len')!
  const startBtn = root.querySelector<HTMLButtonElement>('.start-btn')!
  const formError = root.querySelector<HTMLDivElement>('.form-error')!

  function getOutputMode(): OutputMode {
    const checked = root.querySelector<HTMLInputElement>('input[name="outputMode"]:checked')
    return (checked?.value as OutputMode) ?? 'same_as_source'
  }

  outputModeRadios.forEach((radio) => {
    radio.addEventListener('change', () => {
      outputDirInput.disabled = getOutputMode() !== 'custom'
    })
  })

  selectedMedia.subscribe((files) => {
    if (files.length === 0) {
      selectedMediaEl.textContent = 'No files selected'
    } else if (files.length > 5) {
      selectedMediaEl.textContent = `${files.length} files selected`
    } else {
      selectedMediaEl.textContent = files.map((v) => v.name).join(', ')
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

    const files = selectedMedia.get()
    if (files.length === 0) {
      formError.textContent = 'Select at least one file on the left first'
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
        files.map((file) =>
          api.createJob({
            mediaPath: file.path,
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
          newTabs.push({ id: result.value.jobId, mediaName: files[i].name })
        } else {
          const message = result.reason instanceof Error ? result.reason.message : String(result.reason)
          errors.push(`${files[i].name}: ${message}`)
        }
      })

      if (newTabs.length > 0) {
        openTabs.set([...openTabs.get(), ...newTabs])
        activeTabId.set(newTabs[0].id)
        onJobCreated()
      }
      formError.textContent = errors.join('; ')
      selectedMedia.set([])
    } finally {
      startBtn.disabled = false
    }
  })

  loadModels()
  loadLanguages()

  return { refreshModels: loadModels }
}
