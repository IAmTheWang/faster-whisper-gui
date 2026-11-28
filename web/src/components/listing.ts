import type { Drive, Entry } from '../api'
import { escapeHtml } from '../dom'

// Helpers shared by the two file listings — directoryBrowser (media picker) and
// dirPicker (settings/save folder picker) — which used to carry copies of each.

// Sequencer guards against out-of-order async responses: take a ticket before
// starting a request, and only apply the result if it is still the newest.
// invalidate() drops every request still in flight.
export class Sequencer {
  private latest = 0

  next(): number {
    return ++this.latest
  }

  isCurrent(ticket: number): boolean {
    return ticket === this.latest
  }

  invalidate(): void {
    this.latest++
  }
}

const ICONS: Record<Entry['type'], string> = {
  dir: '📁',
  video: '🎬',
  audio: '🎵',
  file: '📄',
}

export function entryIcon(type: Entry['type']): string {
  return ICONS[type] ?? ICONS.file
}

// Emoji are decorative: hide them from screen readers so a row is announced as
// its file name, not "clapper board Foo.mp4".
export function createEntryLabel(entry: Entry): HTMLSpanElement {
  const label = document.createElement('span')
  const icon = document.createElement('span')
  icon.className = 'entry-icon'
  icon.setAttribute('aria-hidden', 'true')
  icon.textContent = entryIcon(entry.type)
  label.append(icon, ` ${entry.name}`)
  return label
}

// keepFocusInList puts keyboard focus back into a list that was just rebuilt
// (rebuilding drops the focused row to <body>, which for a dialog also loses
// Escape handling): the first row's control, or the list itself when empty.
export function keepFocusInList(list: HTMLElement, hadFocus: boolean): void {
  if (!hadFocus) return
  list.tabIndex = -1
  ;(list.querySelector<HTMLElement>('.row-btn, .media-checkbox') ?? list).focus()
}

export function renderListError(list: HTMLElement, message: string): void {
  list.innerHTML = `<li class="entry-error" role="alert">${escapeHtml(message)}</li>`
}

export function renderListEmpty(list: HTMLElement): void {
  list.innerHTML = '<li class="entry-empty">(empty)</li>'
}

// driveFor finds the drive a path lives on ("e:\videos" -> the "E:\" drive).
export function driveFor(drives: Drive[], path: string): Drive | undefined {
  const upper = path.toUpperCase()
  return drives.find((d) => upper.startsWith(d.name.toUpperCase()))
}
