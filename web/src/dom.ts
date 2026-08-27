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
