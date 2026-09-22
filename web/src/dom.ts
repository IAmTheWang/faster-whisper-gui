// escapeHtml guards template-string innerHTML interpolation of
// filesystem-derived strings (filenames, paths). Windows filenames can't
// actually contain '<' or '>', so this isn't defending against a realistic
// XSS payload here — it's cheap insurance against '&' and friends breaking
// entity parsing, and against relying on that platform detail at all.
export function escapeHtml(value: string): string {
  const div = document.createElement('div')
  div.textContent = value
  return div.innerHTML
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
