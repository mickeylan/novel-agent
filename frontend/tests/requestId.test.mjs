import assert from 'node:assert/strict'
import test from 'node:test'

import { attachRequestID } from '../src/api/requestId.js'

const firstId = '123e4567-e89b-12d3-a456-426614174000'
const secondId = '223e4567-e89b-12d3-a456-426614174000'

test('attaches a valid request UUID without mutating other headers', () => {
  const config = { headers: { Accept: 'application/json' } }
  const result = attachRequestID(config, () => firstId)
  assert.equal(result, config)
  assert.equal(config.headers.Accept, 'application/json')
  assert.equal(config.headers['X-Request-ID'], firstId)
})

test('preserves an explicitly supplied valid request ID case-insensitively', () => {
  const config = { headers: { 'x-request-id': firstId } }
  attachRequestID(config, () => secondId)
  assert.deepEqual(config.headers, { 'x-request-id': firstId })
})

test('replaces malformed explicit values only when a valid UUID can be generated', () => {
  const config = { headers: { 'x-request-id': 'bad\nvalue' } }
  attachRequestID(config, () => secondId)
  assert.equal(config.headers['X-Request-ID'], secondId)
  assert.equal(config.headers['x-request-id'], undefined)

  const unavailable = { headers: {} }
  attachRequestID(unavailable, () => undefined)
  assert.equal(unavailable.headers['X-Request-ID'], undefined)
})

test('supports AxiosHeaders-compatible get and set methods', () => {
  const values = new Map()
  const headers = {
    get: name => values.get(name.toLowerCase()),
    set: (name, value) => values.set(name.toLowerCase(), value)
  }
  attachRequestID({ headers }, () => firstId)
  assert.equal(values.get('x-request-id'), firstId)
})
