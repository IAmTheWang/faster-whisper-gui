import { describe, expect, it } from 'vitest'
import { basenameOf, dirnameOf, errMsg, escapeHtml, parseBoundedInt, requireEl, stripSurroundingQuotes } from './dom'

describe('escapeHtml', () => {
  it('escapes markup and both quote kinds (used inside attribute values)', () => {
    expect(escapeHtml(`<a href="x" onclick='y'>&`)).toBe(
      '&lt;a href=&quot;x&quot; onclick=&#39;y&#39;&gt;&amp;',
    )
  })
  it('escapes & first so entities are not double-escaped', () => {
    expect(escapeHtml('&lt;')).toBe('&amp;lt;')
  })
})

describe('errMsg', () => {
  it('uses Error.message, falls back to String()', () => {
    expect(errMsg(new Error('boom'))).toBe('boom')
    expect(errMsg('plain')).toBe('plain')
    expect(errMsg(42)).toBe('42')
  })
})

describe('basenameOf', () => {
  it('handles backslash and forward-slash paths', () => {
    expect(basenameOf(String.raw`E:\videos\a.b.mp4`)).toBe('a.b.mp4')
    expect(basenameOf('E:/videos/clip.mkv')).toBe('clip.mkv')
    expect(basenameOf('bare.wav')).toBe('bare.wav')
  })
})

describe('stripSurroundingQuotes', () => {
  it("removes Explorer's Copy-as-path quotes and surrounding whitespace", () => {
    expect(stripSurroundingQuotes(String.raw`  "C:\a b\c.mp4" `)).toBe(String.raw`C:\a b\c.mp4`)
    expect(stripSurroundingQuotes(String.raw`'D:\x'`)).toBe(String.raw`D:\x`)
    expect(stripSurroundingQuotes(String.raw`C:\plain`)).toBe(String.raw`C:\plain`)
  })
})

describe('dirnameOf', () => {
  it('returns the parent directory', () => {
    expect(dirnameOf(String.raw`C:\videos\a.mp4`)).toBe(String.raw`C:\videos`)
    expect(dirnameOf('C:/videos/a.mp4')).toBe('C:/videos')
  })
  it('keeps the root slash for a drive-root file (bare "C:" means cwd on C)', () => {
    expect(dirnameOf(String.raw`C:\a.mp4`)).toBe('C:' + String.fromCharCode(92))
  })
  it('returns null when there is no parent', () => {
    expect(dirnameOf('a.mp4')).toBeNull()
  })
})

describe('parseBoundedInt', () => {
  it('returns undefined for blank input, null for invalid, else the integer', () => {
    expect(parseBoundedInt('', 0, 10)).toBeUndefined()
    expect(parseBoundedInt('  ', 0, 10)).toBeUndefined()
    expect(parseBoundedInt('5', 0, 10)).toBe(5)
    expect(parseBoundedInt(' -1 ', -1, 10)).toBe(-1)
    expect(parseBoundedInt('11', 0, 10)).toBeNull()
    expect(parseBoundedInt('-1', 0, 10)).toBeNull()
    expect(parseBoundedInt('2.5', 0, 10)).toBeNull()
    expect(parseBoundedInt('abc', 0, 10)).toBeNull()
  })
})

describe('requireEl', () => {
  it('returns the element, or throws a readable error when it is missing', () => {
    const el = {}
    expect(requireEl({ querySelector: () => el } as unknown as ParentNode, '.x')).toBe(el)
    expect(() => requireEl({ querySelector: () => null } as unknown as ParentNode, '.missing')).toThrow(
      'Missing UI element: .missing',
    )
  })
})
