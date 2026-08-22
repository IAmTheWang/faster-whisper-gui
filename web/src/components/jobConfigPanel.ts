import { api, type Language, type Model, type OutputMode } from '../api'
import { activeJobId, selectedVideo } from '../state'

export function mountJobConfigPanel(root: HTMLElement, onJobCreated: () => void): void {
  root.innerHTML = `
    <div class="panel">
      <h2>转录设置</h2>
      <div class="field">
        <label>视频文件</label>
        <div class="selected-video">未选择</div>
      </div>
      <div class="field">
        <label for="model-select">模型</label>
        <select id="model-select" class="model-select"></select>
      </div>
      <div class="field">
        <label for="language-select">语言</label>
        <select id="language-select" class="language-select"></select>
      </div>
      <div class="field">
        <label>输出位置</label>
        <label class="radio-label">
          <input type="radio" name="outputMode" value="same_as_video" checked /> 默认（与视频同目录）
        </label>
        <label class="radio-label">
          <input type="radio" name="outputMode" value="custom" /> 自定义目录
        </label>
        <input type="text" class="output-dir" placeholder="E:\\subtitles" disabled />
      </div>
      <details class="advanced">
        <summary>高级选项</summary>
        <div class="field">
          <label for="max-len">单条字幕最大字符数（留空使用默认）</label>
          <input type="number" id="max-len" class="max-len" min="1" />
        </div>
      </details>
      <button type="button" class="btn-primary start-btn">开始转录</button>
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

  selectedVideo.subscribe((video) => {
    selectedVideoEl.textContent = video ? video.path : '未选择'
  })

  async function loadOptions(): Promise<void> {
    const [models, languages] = await Promise.all([api.models(), api.languages()])
    modelSelect.innerHTML = models.map((m: Model) => `<option value="${m.id}">${m.label}</option>`).join('')
    languageSelect.innerHTML = languages
      .map((l: Language) => `<option value="${l.code}">${l.label}</option>`)
      .join('')
    if (models.length === 0) {
      formError.textContent = '未检测到任何模型文件，请将 ggml-*.bin 放入 models 目录后刷新页面'
    }
  }

  startBtn.addEventListener('click', async () => {
    formError.textContent = ''

    const video = selectedVideo.get()
    if (!video) {
      formError.textContent = '请先在左侧选择一个视频文件'
      return
    }
    if (!modelSelect.value) {
      formError.textContent = '请先选择模型'
      return
    }

    const outputMode = getOutputMode()
    const maxLenValue = maxLenInput.value.trim()

    startBtn.disabled = true
    try {
      const res = await api.createJob({
        videoPath: video.path,
        modelId: modelSelect.value,
        language: languageSelect.value || 'auto',
        outputMode,
        outputDir: outputMode === 'custom' ? outputDirInput.value.trim() : undefined,
        maxLen: maxLenValue ? Number(maxLenValue) : undefined,
      })
      activeJobId.set(res.jobId)
      onJobCreated()
    } catch (err) {
      formError.textContent = err instanceof Error ? err.message : String(err)
    } finally {
      startBtn.disabled = false
    }
  })

  loadOptions()
}
