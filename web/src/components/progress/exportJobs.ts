import { basenameOf, errMsg } from '../../dom'
import type { TabInfo } from '../../state'
import type { TabState } from './tabState'

export interface ExportOutcome {
  succeeded: number
  total: number
  problems: string[]
}

type ExportFn = (jobId: string, destDir: string) => Promise<unknown>
type StateLookup = (jobId: string) => Pick<TabState, 'status' | 'srtPath'> | undefined

// exportCompleted copies every completed tab's .srt into destDir, one at a
// time. Two jobs whose .srt files share a basename would overwrite each other
// in one folder, so a later one is skipped and reported instead — but only a
// copy that actually succeeded claims its name, so a failed export doesn't
// block the next job with the same name.
export async function exportCompleted(
  exportJob: ExportFn,
  tabs: readonly TabInfo[],
  stateOf: StateLookup,
  destDir: string,
): Promise<ExportOutcome> {
  const doneTabs = tabs.filter((t) => stateOf(t.id)?.status === 'done')
  const problems: string[] = []
  const claimed = new Map<string, string>() // lower-cased basename -> media name that took it
  let succeeded = 0
  for (const tab of doneTabs) {
    const srtPath = stateOf(tab.id)?.srtPath
    const base = srtPath ? basenameOf(srtPath).toLowerCase() : ''
    const owner = base ? claimed.get(base) : undefined
    if (owner !== undefined) {
      problems.push(`${tab.mediaName}: skipped, "${basenameOf(srtPath!)}" would overwrite the copy from ${owner}`)
      continue
    }
    try {
      await exportJob(tab.id, destDir)
      if (base) claimed.set(base, tab.mediaName)
      succeeded++
    } catch (err) {
      problems.push(`${tab.mediaName}: ${errMsg(err)}`)
    }
  }
  return { succeeded, total: doneTabs.length, problems }
}

// summarizeExport is the "Saved N/M" line shown after Save All Completed.
export function summarizeExport(o: ExportOutcome): { text: string; kind: 'success' | 'error' } {
  return o.problems.length === 0
    ? { text: `Saved ${o.succeeded}/${o.total}`, kind: 'success' }
    : { text: `Saved ${o.succeeded}/${o.total} — ${o.problems.join('; ')}`, kind: 'error' }
}
