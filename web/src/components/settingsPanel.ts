import { api, type UpdateSettingsRequest } from '../api'
import { dirnameOf, errMsg, requireEl } from '../dom'
import { mountDirPicker, type PickerKind } from './dirPicker'

type SettingsField = 'ffmpegPath' | 'whisperCliPath' | 'modelsDir' | 'defaultVideoDir'

const SETTINGS_FIELDS: readonly SettingsField[] = ['ffmpegPath', 'whisperCliPath', 'modelsDir', 'defaultVideoDir']

function isSettingsField(value: unknown): value is SettingsField {
  return (SETTINGS_FIELDS as readonly unknown[]).includes(value)
}

function isPickerKind(value: unknown): value is PickerKind {
  return value === 'exe' || value === 'dir'
}

// mountSettingsPanel lets the user override where ffmpeg.exe, whisper-cli.exe,
// and the models directory live, instead of requiring them under
// bin/ffmpeg, bin/whisper, models next to the program (see internal/config's
// Effective* methods and internal/settings). onSaved is called after a
// successful save/reset so main.ts can re-probe health and refresh the
// model dropdown.
export function mountSettingsPanel(root: HTMLElement, onSaved: () => void): void {
  root.innerHTML = `
    <div class="panel">
      <h2>Environment Settings</h2>
      <div class="field">
        <label for="setting-ffmpegPath">ffmpeg.exe Path</label>
        <div class="settings-row">
          <input type="text" id="setting-ffmpegPath" class="settings-input" data-field="ffmpegPath" placeholder="Use default path" />
          <button type="button" class="btn-secondary browse-btn" data-field="ffmpegPath" data-kind="exe">Browse</button>
        </div>
      </div>
      <div class="field">
        <label for="setting-whisperCliPath">whisper-cli.exe Path</label>
        <div class="settings-row">
          <input type="text" id="setting-whisperCliPath" class="settings-input" data-field="whisperCliPath" placeholder="Use default path" />
          <button type="button" class="btn-secondary browse-btn" data-field="whisperCliPath" data-kind="exe">Browse</button>
        </div>
      </div>
      <div class="field">
        <label for="setting-modelsDir">Models Directory</label>
        <div class="settings-row">
          <input type="text" id="setting-modelsDir" class="settings-input" data-field="modelsDir" placeholder="Use default path" />
          <button type="button" class="btn-secondary browse-btn" data-field="modelsDir" data-kind="dir">Browse</button>
        </div>
      </div>
      <div class="field">
        <label for="setting-defaultVideoDir">Default Media Directory</label>
        <div class="settings-row">
          <input type="text" id="setting-defaultVideoDir" class="settings-input" data-field="defaultVideoDir" placeholder="Not set — lists all drives by default" />
          <button type="button" class="btn-secondary browse-btn" data-field="defaultVideoDir" data-kind="dir">Browse</button>
        </div>
      </div>
      <div class="picker-mount"></div>
      <div class="settings-actions">
        <button type="button" class="btn-primary save-btn">Save Settings</button>
        <button type="button" class="btn-secondary reset-btn">Reset to Default</button>
      </div>
      <div class="form-error" role="alert"></div>
    </div>
  `

  const inputs: Record<SettingsField, HTMLInputElement> = {
    ffmpegPath: requireEl<HTMLInputElement>(root, 'input[data-field="ffmpegPath"]'),
    whisperCliPath: requireEl<HTMLInputElement>(root, 'input[data-field="whisperCliPath"]'),
    modelsDir: requireEl<HTMLInputElement>(root, 'input[data-field="modelsDir"]'),
    defaultVideoDir: requireEl<HTMLInputElement>(root, 'input[data-field="defaultVideoDir"]'),
  }

  const saveBtn = requireEl<HTMLButtonElement>(root, '.save-btn')
  const resetBtn = requireEl<HTMLButtonElement>(root, '.reset-btn')
  const formError = requireEl<HTMLDivElement>(root, '.form-error')

  const picker = mountDirPicker(requireEl<HTMLDivElement>(root, '.picker-mount'))

  async function loadSettings(): Promise<void> {
    const s = await api.settings()
    inputs.ffmpegPath.value = s.ffmpegPath
    inputs.ffmpegPath.placeholder = s.ffmpegDefault ? `Default: ${s.ffmpegDefault}` : 'No default path detected'
    inputs.whisperCliPath.value = s.whisperCliPath
    inputs.whisperCliPath.placeholder = s.whisperCliDefault ? `Default: ${s.whisperCliDefault}` : 'No default path detected'
    inputs.modelsDir.value = s.modelsDir
    inputs.modelsDir.placeholder = `Default: ${s.modelsDirDefault}`
    inputs.defaultVideoDir.value = s.defaultVideoDir
  }

  function openPicker(field: SettingsField, kind: PickerKind): void {
    formError.textContent = ''
    const current = inputs[field].value.trim()
    void picker.open({
      kind,
      initialPath: kind === 'exe' ? (dirnameOf(current) ?? undefined) : current || undefined,
      onSelect: (path) => {
        inputs[field].value = path
      },
    })
  }

  root.querySelectorAll<HTMLButtonElement>('.browse-btn').forEach((btn) => {
    btn.addEventListener('click', () => {
      const { field, kind } = btn.dataset
      if (isSettingsField(field) && isPickerKind(kind)) openPicker(field, kind)
    })
  })

  async function submit(body: UpdateSettingsRequest, busyBtn: HTMLButtonElement, busyLabel: string): Promise<void> {
    formError.textContent = ''
    const originalLabel = busyBtn.textContent
    saveBtn.disabled = true
    resetBtn.disabled = true
    busyBtn.textContent = busyLabel
    try {
      await api.updateSettings(body)
      await loadSettings()
      onSaved()
    } catch (err) {
      formError.textContent = errMsg(err)
    } finally {
      saveBtn.disabled = false
      resetBtn.disabled = false
      busyBtn.textContent = originalLabel
    }
  }

  saveBtn.addEventListener('click', () =>
    submit(
      {
        ffmpegPath: inputs.ffmpegPath.value.trim(),
        whisperCliPath: inputs.whisperCliPath.value.trim(),
        modelsDir: inputs.modelsDir.value.trim(),
        defaultVideoDir: inputs.defaultVideoDir.value.trim(),
      },
      saveBtn,
      'Saving...',
    ),
  )

  resetBtn.addEventListener('click', () =>
    submit(
      { ffmpegPath: '', whisperCliPath: '', modelsDir: '', defaultVideoDir: '' },
      resetBtn,
      'Resetting...',
    ),
  )

  loadSettings().catch((err) => {
    formError.textContent = `Could not load settings: ${errMsg(err)}`
  })
}
