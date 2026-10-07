import assert from 'node:assert/strict'
import test from 'node:test'

import { AI_PREVIEW_TIMEOUT_MS, STANDARD_API_TIMEOUT_MS, aiPreviewRequest, withTimeout } from '../src/api/timeouts.js'

test('uses short defaults for ordinary APIs and bounded long AI previews', () => {
  assert.equal(STANDARD_API_TIMEOUT_MS, 30_000)
  assert.equal(AI_PREVIEW_TIMEOUT_MS, 1_200_000)
  assert.deepEqual(aiPreviewRequest, { timeout: AI_PREVIEW_TIMEOUT_MS })
  assert.equal(Object.isFrozen(aiPreviewRequest), true)
})

test('rejects invalid or unbounded timeout overrides', () => {
  assert.throws(() => withTimeout(0), RangeError)
  assert.throws(() => withTimeout(AI_PREVIEW_TIMEOUT_MS + 1), RangeError)
  assert.throws(() => withTimeout(1.5), RangeError)
  assert.deepEqual(withTimeout(45_000), { timeout: 45_000 })
})
