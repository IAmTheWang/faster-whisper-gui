export interface JobFailure {
  name: string
  message: string
}

const SHOWN_NAMES = 3

// summarizeFailures turns per-file creation errors into one readable line.
// Failures that share a message are grouped ("3 files: msg (a.mp4, b.mp4, ...)"):
// a batch of 80 files hitting the same problem (a missing binary, a full queue)
// would otherwise produce 80 near-identical fragments.
export function summarizeFailures(failures: JobFailure[]): string {
  const groups = new Map<string, string[]>()
  for (const f of failures) {
    const names = groups.get(f.message)
    if (names) names.push(f.name)
    else groups.set(f.message, [f.name])
  }
  const parts: string[] = []
  for (const [message, names] of groups) {
    if (names.length === 1) {
      parts.push(`${names[0]}: ${message}`)
      continue
    }
    const shown = names.slice(0, SHOWN_NAMES).join(', ')
    const more = names.length > SHOWN_NAMES ? ', …' : ''
    parts.push(`${names.length} files: ${message} (${shown}${more})`)
  }
  return parts.join('; ')
}
