import assert from 'node:assert/strict'
import test from 'node:test'

import { buildDraftFilename, buildDraftText } from '../src/utils/draftExport.js'

test('buildDraftFilename creates a filesystem-safe deterministic name', () => {
  const name = buildDraftFilename(12, new Date('2026-10-07T01:40:30.123Z'))
  assert.equal(name, 'chapter-12_local-draft_2026-10-07_01-40-30-123.txt')
  assert.equal(/[<>:"/\\|?*]/.test(name), false)
})

test('buildDraftText preserves Unicode and records the loaded version', () => {
  const text = buildDraftText(3, '中文正文🙂', '2026-10-07T01:30:00Z')
  assert.equal(text.startsWith('\uFEFF'), true)
  assert.match(text, /第 3 章/)
  assert.match(text, /2026-10-07T01:30:00Z/)
  assert.match(text, /中文正文🙂/)
})
