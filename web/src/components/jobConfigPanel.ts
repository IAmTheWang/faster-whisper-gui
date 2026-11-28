import { api, type OutputMode } from '../api'
import { errMsg, parseBoundedInt, requireEl } from '../dom'
import { activeTabId, openTabs, selectedMedia, type TabInfo } from '../state'
import { summarizeFailures } from './jobFailures'

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
        <input type="text" id="output-dir" class="output-dir" aria-label="Custom output directory" placeholder="E:\\subtitles" disabled />
      </div>
      <details class="advanced">
        <summary>Advanced Options</summary>
        <div class="field">
          <label for="max-len">Max characters per subtitle line (leave blank for default)</label>
          <input type="number" id="max-len" class="max-len" min="0" max="1000" step="1" />
        </div>
        <div class="field">
          <label for="max-context">Context carried between segments (0 = disabled, recommended; -1 = whisper default/unlimited)</label>
          <input type="number" id="max-context" class="max-context" value="0" min="-1" max="4096" step="1" />
        </div>
      </details>
      <button type="button" class="btn-primary start-btn">Start Transcription</button>
      <div class="form-error" role="alert"></div>
    </div>
  `

  const selectedMediaEl = requireEl<HTMLDivElement>(root, '.selected-media')
  const modelSelect = requireEl<HTMLSelectElement>(root, '.model-select')
  const languageSelect = requireEl<HTMLSelectElement>(root, '.language-select')
  const outputModeRadios = root.querySelectorAll<HTMLInputElement>('input[name="outputMode"]')
  const outputDirInput = requireEl<HTMLInputElement>(root, '.output-dir')
  const maxLenInput = requireEl<HTMLInputElement>(root, '.max-len')
  const maxContextInput = requireEl<HTMLInputElement>(root, '.max-context')
  const startBtn = requireEl<HTMLButtonElement>(root, '.start-btn')
  const formError = requireEl<HTMLDivElement>(root, '.form-error')

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

  // Text of the last "no models" notice, so a later successful reload only
  // clears that notice and not an unrelated submit error.
  let modelsNotice = ''

  function setOptions(select: HTMLSelectElement, items: { value: string; label: string }[]): void {
    select.replaceChildren(
      ...items.map((item) => {
        const opt = document.createElement('option')
        opt.value = item.value
        opt.textContent = item.label
        return opt
      }),
    )
  }

  async function loadModels(): Promise<void> {
    try {
      const models = await api.models()
      const previous = modelSelect.value
      setOptions(
        modelSelect,
        models.map((m) => ({ value: m.id, label: m.label })),
      )
      // Keep the user's choice across a rebuild (e.g. after a Settings save).
      if (previous && models.some((m) => m.id === previous)) modelSelect.value = previous
      if (models.length === 0) {
        modelsNotice = 'No model files detected — put a ggml-*.bin file in the models directory and refresh'
        formError.textContent = modelsNotice
      } else if (modelsNotice && formError.textContent === modelsNotice) {
        modelsNotice = ''
        formError.textContent = ''
      }
    } catch (err) {
      modelsNotice = `Could not load models: ${errMsg(err)}`
      formError.textContent = modelsNotice
    }
  }

  async function loadLanguages(): Promise<void> {
    try {
      const languages = await api.languages()
      setOptions(
        languageSelect,
        languages.map((l) => ({ value: l.code, label: l.label })),
      )
    } catch (err) {
      formError.textContent = `Could not load languages: ${errMsg(err)}`
    }
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
    const language = languageSelect.value || 'auto'
    const outputDir = outputMode === 'custom' ? outputDirInput.value.trim() : undefined
    const maxLen = parseBoundedInt(maxLenInput.value, 0, 1000)
    if (maxLen === null) {
      formError.textContent = 'Max characters per line must be a whole number between 0 and 1000 (or blank)'
      return
    }
    const maxContextParsed = parseBoundedInt(maxContextInput.value, -1, 4096)
    if (maxContextParsed === null) {
      formError.textContent = 'Context must be a whole number between -1 and 4096'
      return
    }
    const maxContext = maxContextParsed ?? 0
    const modelId = modelSelect.value

    startBtn.disabled = true
    try {
      // One request at a time, in selection order, so jobs queue in the order
      // the user picked them.
      const newTabs: TabInfo[] = []
      const createdPaths = new Set<string>()
      const failures: { name: string; message: string }[] = []
      for (const file of files) {
        try {
          const res = await api.createJob({
            mediaPath: file.path,
            modelId,
            language,
            outputMode,
            outputDir,
            maxLen,
            maxContext,
          })
          newTabs.push({ id: res.jobId, mediaName: file.name })
          createdPaths.add(file.path)
        } catch (err) {
          failures.push({ name: file.name, message: errMsg(err) })
        }
      }

      const firstNewTab = newTabs[0]
      if (firstNewTab) {
        openTabs.set([...openTabs.get(), ...newTabs])
        activeTabId.set(firstNewTab.id)
        onJobCreated()
      }
      formError.textContent = summarizeFailures(failures)
      // Only drop files that actually became jobs; failures stay checked so
      // the user can fix the problem and retry. (Re-read the store: the user
      // may have changed the selection while requests were in flight.)
      selectedMedia.set(selectedMedia.get().filter((f) => !createdPaths.has(f.path)))
    } finally {
      startBtn.disabled = false
    }
  })

  loadModels()
  loadLanguages()

  return { refreshModels: loadModels }
}
