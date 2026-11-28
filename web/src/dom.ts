// escapeHtml guards template-string innerHTML interpolation of
// filesystem-derived strings (filenames, paths), including inside quoted
// attribute values — hence the quote escapes. Windows filenames can't
// actually contain '<' or '>', so this isn't defending against a realistic
// XSS payload here — it's cheap insurance against '&' and friends breaking
// entity parsing, and against relying on that platform detail at all.
export function escapeHtml(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;')
}

// errMsg turns anything thrown (an Error from api.ts, or a stray string)
// into display text.
export function errMsg(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

// basenameOf extracts the filename from a Windows or forward-slash path, for
// labeling a Progress tab reopened from Job History (a fresh job already has
// its Entry.name in hand and doesn't need this).
export function basenameOf(path: string): string {
  return path.split(/[\\/]/).pop() ?? path
}

// stripSurroundingQuotes undoes what Explorer's "Copy as path" adds. A bare
// quote can't legally appear in a Windows path, so stripping either end
// unconditionally (not just matched pairs) is safe.
export function stripSurroundingQuotes(raw: string): string {
  return raw.trim().replace(/^["']|["']$/g, '')
}

// dirnameOf mirrors basenameOf: the parent directory of a Windows or
// forward-slash path. Special-cases a bare drive letter (e.g. stripping
// "video.mp4" from "C:\video.mp4" naively yields "C:", which Windows/Go
// treat as "current directory on C:", not the root "C:\").
export function dirnameOf(path: string): string | null {
  const idx = Math.max(path.lastIndexOf('\\'), path.lastIndexOf('/'))
  if (idx <= 0) return null
  const dir = path.slice(0, idx)
  return /^[a-zA-Z]:$/.test(dir) ? `${dir}\\` : dir
}

// requireEl is querySelector for elements a component's own template just
// rendered: a miss is a bug in that template, so it throws a readable error
// instead of leaving a `!` to crash later with "cannot read properties of null".
export function requireEl<T extends Element>(root: ParentNode, selector: string): T {
  const el = root.querySelector<T>(selector)
  if (!el) throw new Error(`Missing UI element: ${selector}`)
  return el
}

// parseBoundedInt returns undefined for blank input (caller decides the
// default), null if invalid (not a whole number, or outside min..max), else the
// integer.
export function parseBoundedInt(raw: string, min: number, max: number): number | null | undefined {
  const trimmed = raw.trim()
  if (trimmed === '') return undefined
  const n = Number(trimmed)
  if (!Number.isInteger(n) || n < min || n > max) return null
  return n
}

// rowButton is the focusable control of a clickable list row: a real <button>
// inside the <li>, so the row keeps its list semantics while keyboard and
// assistive-tech users get native button behaviour (Enter/Space fire a click).
export function rowButton(): HTMLButtonElement {
  const btn = document.createElement('button')
  btn.type = 'button'
  btn.className = 'row-btn'
  return btn
}

// delegateActivate wires ONE click listener on container for every current and
// future descendant matching selector, instead of one per row (lists are
// rebuilt wholesale on every refresh). A row's button turns Enter/Space into a
// click, so keys need no handling of their own. Clicks whose target matches
// ignoreSelector (e.g. a checkbox with its own handler) are skipped.
export function delegateActivate(
  container: HTMLElement,
  selector: string,
  onActivate: (el: HTMLElement) => void,
  ignoreSelector?: string,
): void {
  container.addEventListener('click', (e) => {
    const target = e.target as Element | null
    if (!target || (ignoreSelector && target.closest(ignoreSelector))) return
    const el = target.closest<HTMLElement>(selector)
    if (el && container.contains(el)) onActivate(el)
  })
}
