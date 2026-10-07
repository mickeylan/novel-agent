import assert from 'node:assert/strict'
import test from 'node:test'

import { formatApiError, normalizeApiError } from '../src/api/error.js'

const requestId = '123e4567-e89b-12d3-a456-426614174000'

test('normalizes stable API metadata without exposing arbitrary values', () => {
  const detail = normalizeApiError({
    response: {
      status: 429,
      headers: { 'x-request-id': requestId, 'retry-after': '1' },
      data: { code: 'ai_capacity_exceeded', error: 'AI request capacity exceeded' }
    }
  })
  assert.deepEqual(detail, {
    status: 429,
    code: 'ai_capacity_exceeded',
    requestId,
    retryAfter: 1,
    retrySafe: true,
    message: 'AI request capacity exceeded'
  })
})

test('formats capacity errors with retry and correlation details', () => {
  const message = formatApiError({ apiError: {
    status: 429,
    code: 'ai_capacity_exceeded',
    requestId,
    retryAfter: 1,
    message: 'AI request capacity exceeded'
  } })
  assert.equal(message, `AI 服务繁忙，请稍后重试（建议等待 1 秒）；请求 ID：${requestId}`)
})

test('warns against retrying an ambiguous committed write', () => {
  const detail = normalizeApiError({
    response: {
      status: 500,
      headers: { 'x-request-id': requestId },
      data: { code: 'write_result_unavailable', error: 'write may have succeeded but the saved result could not be loaded', retry_safe: false }
    }
  })
  assert.equal(detail.retrySafe, false)
  assert.equal(formatApiError({ apiError: detail }), `写入可能已成功，但无法读取保存结果。请先刷新核对服务器数据，不要直接重试；请求 ID：${requestId}`)
})

test('formats invalid chapter path errors consistently', () => {
  const message = formatApiError({ apiError: {
    status: 400,
    code: 'invalid_chapter_number',
    requestId,
    retryAfter: null,
    retrySafe: true,
    message: 'chapter_number must be an integer between 1 and 1000000'
  } })
  assert.equal(message, `章节号无效，必须是 1–1000000 之间的整数；请求 ID：${requestId}`)
})

test('formats invalid UUID path errors consistently', () => {
  for (const [code, expected] of [
    ['invalid_project_id', '项目 ID 格式无效'],
    ['invalid_character_id', '角色 ID 格式无效']
  ]) {
    const message = formatApiError({ apiError: {
      status: 400,
      code,
      requestId,
      retryAfter: null,
      retrySafe: true,
      message: 'invalid UUID'
    } })
    assert.equal(message, `${expected}；请求 ID：${requestId}`)
  }
})

test('formats duplicate JSON key errors without exposing field names', () => {
  const message = formatApiError({ apiError: {
    status: 400,
    code: 'duplicate_json_key',
    requestId,
    retryAfter: null,
    retrySafe: true,
    message: 'request body contains duplicate JSON object keys'
  } })
  assert.equal(message, `请求包含重复字段，请刷新页面后重试；请求 ID：${requestId}`)
})

test('formats excessive JSON nesting errors consistently', () => {
  const message = formatApiError({ apiError: {
    status: 400,
    code: 'json_nesting_too_deep',
    requestId,
    retryAfter: null,
    retrySafe: true,
    message: 'request body exceeds the JSON nesting depth limit'
  } })
  assert.equal(message, `请求结构嵌套过深，请简化后重试；请求 ID：${requestId}`)
})

test('preserves the sent correlation ID when a client timeout has no response', () => {
  const detail = normalizeApiError({
    code: 'ECONNABORTED',
    config: { headers: { 'X-Request-ID': requestId } }
  })
  assert.equal(detail.status, 0)
  assert.equal(detail.requestId, requestId)
  assert.equal(detail.message, '请求超时，服务端可能仍在处理')
  assert.equal(formatApiError({ apiError: detail }), `请求超时，服务端可能仍在处理；请求 ID：${requestId}`)
})

test('does not display malformed request IDs or oversized server errors', () => {
  const detail = normalizeApiError({
    message: 'Network Error',
    response: {
      status: 500,
      headers: { 'x-request-id': 'bad\nvalue' },
      data: { code: '../../bad', error: 'x'.repeat(301), error_id: 'also-bad' }
    }
  })
  assert.equal(detail.code, 'request_failed')
  assert.equal(detail.requestId, '')
  assert.equal(detail.message, '网络连接失败')
  assert.equal(formatApiError({ apiError: detail }, '操作失败'), '网络连接失败')
})
