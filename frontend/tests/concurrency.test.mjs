import assert from 'node:assert/strict'
import test from 'node:test'

import { isSameServerVersion, isUpdateConflict, withUpdatedAt } from '../src/api/concurrency.js'

test('withUpdatedAt adds the loaded version without mutating input', () => {
  const input = { title: 'new title' }
  const result = withUpdatedAt(input, '2026-10-05T06:00:00Z')
  assert.deepEqual(input, { title: 'new title' })
  assert.equal(result.updated_at, '2026-10-05T06:00:00Z')
})

test('withUpdatedAt rejects unsafe updates without a version', () => {
  assert.throws(() => withUpdatedAt({ title: 'unsafe' }, ''), /updated_at is required/)
})

test('isUpdateConflict recognizes only explicit optimistic-lock conflicts', () => {
  assert.equal(isUpdateConflict({ response: { status: 409, data: { code: 'update_conflict' } } }), true)
  assert.equal(isUpdateConflict({ response: { status: 409, data: { code: 'other' } } }), false)
  assert.equal(isUpdateConflict({ response: { status: 500, data: { code: 'update_conflict' } } }), false)
})

test('isSameServerVersion requires two identical non-empty versions', () => {
  const version = '2026-10-07T01:36:00Z'
  assert.equal(isSameServerVersion(version, version), true)
  assert.equal(isSameServerVersion(version, '2026-10-07T01:37:00Z'), false)
  assert.equal(isSameServerVersion('', version), false)
  assert.equal(isSameServerVersion(version, null), false)
})
