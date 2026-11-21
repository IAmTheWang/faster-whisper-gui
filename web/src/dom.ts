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

// makeActivatable makes a non-button element (a clickable <li> row) behave
// like a button for keyboard and assistive-tech users: focusable, announced
// as a button, and triggered by Enter/Space as well as click.
export function makeActivatable(el: HTMLElement, onActivate: () => void): void {
  el.setAttribute('role', 'button')
  el.tabIndex = 0
  el.addEventListener('click', onActivate)
  el.addEventListener('keydown', (e) => {
    if (e.target !== el) return
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault()
      onActivate()
    }
  })
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

// delegateActivate wires ONE click and ONE Enter/Space listener on container for
// every current and future descendant matching selector, instead of a pair of
// listeners per row (lists are rebuilt wholesale on every refresh). Clicks whose
// target matches ignoreSelector (e.g. a checkbox with its own handler) are
// skipped; key presses only count when the matching element itself has focus.
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
  container.addEventListener('keydown', (e) => {
    if (e.key !== 'Enter' && e.key !== ' ') return
    const el = (e.target as Element | null)?.closest<HTMLElement>(selector)
    if (el && e.target === el && container.contains(el)) {
      e.preventDefault()
      onActivate(el)
    }
  })
}

// markActivatable gives a non-button element button semantics for keyboard and
// assistive tech (pair it with delegateActivate for the handlers).
export function markActivatable(el: HTMLElement): void {
  el.setAttribute('role', 'button')
  el.tabIndex = 0
}
