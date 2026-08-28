import { api, type BrowseKind, type Entry, type UpdateSettingsRequest } from '../api'
import { escapeHtml } from '../dom'

type SettingsField = 'ffmpegPath' | 'whisperCliPath' | 'modelsDir' | 'defaultVideoDir'

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
        <label>ffmpeg.exe Path</label>
        <div class="settings-row">
          <input type="text" class="settings-input" data-field="ffmpegPath" placeholder="Use default path" />
          <button type="button" class="btn-secondary browse-btn" data-field="ffmpegPath" data-kind="exe">Browse</button>
        </div>
      </div>
      <div class="field">
        <label>whisper-cli.exe Path</label>
        <div class="settings-row">
          <input type="text" class="settings-input" data-field="whisperCliPath" placeholder="Use default path" />
          <button type="button" class="btn-secondary browse-btn" data-field="whisperCliPath" data-kind="exe">Browse</button>
        </div>
      </div>
      <div class="field">
        <label>Models Directory</label>
        <div class="settings-row">
          <input type="text" class="settings-input" data-field="modelsDir" placeholder="Use default path" />
          <button type="button" class="btn-secondary browse-btn" data-field="modelsDir" data-kind="dir">Browse</button>
        </div>
      </div>
      <div class="field">
        <label>Default Media Directory</label>
        <div class="settings-row">
          <input type="text" class="settings-input" data-field="defaultVideoDir" placeholder="Not set — lists all drives by default" />
          <button type="button" class="btn-secondary browse-btn" data-field="defaultVideoDir" data-kind="dir">Browse</button>
        </div>
      </div>
      <div class="settings-picker" hidden>
        <div class="browser-toolbar">
          <select class="drive-select"></select>
        </div>
        <div class="breadcrumb">
          <button type="button" class="btn-link picker-up" hidden>.. Up</button>
          <span class="current-path"></span>
        </div>
        <ul class="entry-list picker-entry-list"></ul>
        <div class="settings-picker-actions">
          <button type="button" class="btn-primary picker-select-dir" hidden>Select This Directory</button>
          <button type="button" class="btn-link picker-cancel">Cancel</button>
        </div>
      </div>
      <div class="settings-actions">
        <button type="button" class="btn-primary save-btn">Save Settings</button>
        <button type="button" class="btn-secondary reset-btn">Reset to Default</button>
      </div>
      <div class="form-error"></div>
    </div>
  `

  const inputs: Record<SettingsField, HTMLInputElement> = {
    ffmpegPath: root.querySelector<HTMLInputElement>('input[data-field="ffmpegPath"]')!,
    whisperCliPath: root.querySelector<HTMLInputElement>('input[data-field="whisperCliPath"]')!,
    modelsDir: root.querySelector<HTMLInputElement>('input[data-field="modelsDir"]')!,
    defaultVideoDir: root.querySelector<HTMLInputElement>('input[data-field="defaultVideoDir"]')!,
  }

  const saveBtn = root.querySelector<HTMLButtonElement>('.save-btn')!
  const resetBtn = root.querySelector<HTMLButtonElement>('.reset-btn')!
  const formError = root.querySelector<HTMLDivElement>('.form-error')!

  const picker = root.querySelector<HTMLDivElement>('.settings-picker')!
  const driveSelect = picker.querySelector<HTMLSelectElement>('.drive-select')!
  const upBtn = picker.querySelector<HTMLButtonElement>('.picker-up')!
  const currentPathEl = picker.querySelector<HTMLSpanElement>('.current-path')!
  const pickerEntryList = picker.querySelector<HTMLUListElement>('.picker-entry-list')!
  const selectDirBtn = picker.querySelector<HTMLButtonElement>('.picker-select-dir')!
  const cancelBtn = picker.querySelector<HTMLButtonElement>('.picker-cancel')!

  let activeField: SettingsField | null = null
  let activeKind: BrowseKind = 'exe'
  let activePath = ''

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

  async function openPicker(field: SettingsField, kind: BrowseKind): Promise<void> {
    activeField = field
    activeKind = kind
    picker.hidden = false
    selectDirBtn.hidden = kind !== 'dir'
    formError.textContent = ''

    const drives = await api.drives()
    driveSelect.innerHTML = drives
      .map((d) => `<option value="${escapeHtml(d.name)}">${escapeHtml(d.name)} ${escapeHtml(d.label)}</option>`)
      .join('')
    if (drives.length > 0) {
      await navigate(drives[0].name)
    }
  }

  async function navigate(path: string): Promise<void> {
    try {
      const listing = await api.browse(path, activeKind)
      activePath = listing.path
      currentPathEl.textContent = listing.path
      upBtn.hidden = !listing.parent
      upBtn.onclick = () => navigate(listing.parent!)
      renderEntries(listing.entries)
    } catch (err) {
      pickerEntryList.innerHTML = `<li class="entry-error">${escapeHtml(err instanceof Error ? err.message : String(err))}</li>`
    }
  }

  function renderEntries(entries: Entry[]): void {
    pickerEntryList.innerHTML = ''
    if (entries.length === 0) {
      pickerEntryList.innerHTML = '<li class="entry-empty">(empty)</li>'
      return
    }
    for (const entry of entries) {
      const li = document.createElement('li')
      li.className = `entry entry-${entry.type}`
      const icon = entry.type === 'dir' ? '📁' : '📄'
      li.textContent = `${icon} ${entry.name}`
      li.addEventListener('click', () => {
        if (entry.type === 'dir') {
          navigate(entry.path)
        } else {
          selectPath(entry.path)
        }
      })
      pickerEntryList.appendChild(li)
    }
  }

  function selectPath(path: string): void {
    if (activeField) {
      inputs[activeField].value = path
    }
    closePicker()
  }

  function closePicker(): void {
    picker.hidden = true
    activeField = null
  }

  driveSelect.addEventListener('change', () => navigate(driveSelect.value))
  selectDirBtn.addEventListener('click', () => selectPath(activePath))
  cancelBtn.addEventListener('click', closePicker)

  root.querySelectorAll<HTMLButtonElement>('.browse-btn').forEach((btn) => {
    btn.addEventListener('click', () => {
      openPicker(btn.dataset.field as SettingsField, btn.dataset.kind as BrowseKind)
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
      formError.textContent = err instanceof Error ? err.message : String(err)
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

  loadSettings()
}
