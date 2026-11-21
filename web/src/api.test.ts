import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './api'

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

describe('api requests', () => {
  it('returns the decoded JSON body', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse([{ code: 'auto', label: 'Auto' }])))
    await expect(api.languages()).resolves.toEqual([{ code: 'auto', label: 'Auto' }])
  })

  it("surfaces the server's error message", async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({ error: 'model not found: x' }, 400)))
    await expect(api.models()).rejects.toThrow('model not found: x')
  })

  it('falls back to the status text when the error body is not JSON', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('<html>', { status: 502, statusText: 'Bad Gateway' })))
    await expect(api.models()).rejects.toThrow('Bad Gateway')
  })

  it('reports an unreadable success body instead of a raw SyntaxError', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('not json', { status: 200 })))
    await expect(api.models()).rejects.toThrow('unreadable response')
  })

  it('sends JSON bodies with a JSON content type', async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ destPath: 'D:\\out\\a.srt' }))
    vi.stubGlobal('fetch', fetchMock)
    await api.exportJob('j 1', 'D:\\out')
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('/api/jobs/j%201/export')
    expect(init.method).toBe('POST')
    expect(new Headers(init.headers).get('Content-Type')).toBe('application/json')
    expect(JSON.parse(init.body as string)).toEqual({ destDir: 'D:\\out' })
  })

  it('gives up on a server that never answers', async () => {
    vi.useFakeTimers()
    vi.stubGlobal(
      'fetch',
      vi.fn(
        (_url: string, init: RequestInit) =>
          new Promise((_resolve, reject) => {
            init.signal?.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')))
          }),
      ),
    )
    const pending = expect(api.health()).rejects.toThrow('did not answer')
    await vi.advanceTimersByTimeAsync(31_000)
    await pending
  })

  it('cancelJob treats a non-2xx answer as an error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(jsonResponse({ error: 'job has already finished' }, 409)))
    await expect(api.cancelJob('j1')).rejects.toThrow('already finished')
  })
})
