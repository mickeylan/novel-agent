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

test('formats missing optimistic-lock versions consistently', () => {
  const message = formatApiError({ apiError: {
    status: 400,
    code: 'invalid_update_version',
    requestId,
    retryAfter: null,
    retrySafe: true,
    message: 'updated_at is required'
  } })
  assert.equal(message, `缺少已加载的服务器版本，请刷新并重新核对内容后再保存；请求 ID：${requestId}`)
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

test('formats domain validation errors without exposing backend wording', () => {
  for (const [code, expected] of [
    ['invalid_project', '项目字段无效或超出长度限制，请检查标题、题材和设定'],
    ['invalid_character', '角色字段无效或超出长度限制，请检查姓名、特征和关系'],
    ['invalid_chapter', '章节字段无效或超出长度限制，请检查标题、大纲和正文'],
    ['invalid_outline', '大纲字段无效，结构必须是大小受限的 JSON 对象'],
    ['invalid_update', '请至少提供一个需要更新的字段'],
    ['invalid_relationships', '角色关系格式无效，请检查后重试']
  ]) {
    const message = formatApiError({ apiError: {
      status: 400,
      code,
      requestId,
      retryAfter: null,
      retrySafe: true,
      message: 'backend wording must not be shown'
    } })
    assert.equal(message, `${expected}；请求 ID：${requestId}`)
  }
})

test('formats bounded resource and routing errors consistently', () => {
  for (const [code, expected] of [
    ['invalid_json_body', '请求内容格式无效，请检查后重试'],
    ['request_body_too_large', '请求内容超过 1 MiB 限制，请缩减后重试'],
    ['token_budget_exceeded', '工作流输出预算不足，请提高预算上限或关闭可选步骤'],
    ['context_unavailable', '写作上下文暂时不可用，请确认项目存在后重试'],
    ['schema_inspection_failed', '数据库结构兼容性检查暂时不可用'],
    ['route_not_found', '请求的接口不存在，请刷新页面后重试'],
    ['method_not_allowed', '当前接口不支持该请求方法，请刷新页面后重试']
  ]) {
    const message = formatApiError({ apiError: {
      status: 400,
      code,
      requestId,
      retryAfter: null,
      retrySafe: true,
      message: 'backend wording must not be shown'
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

test('formats excessive JSON structure errors consistently', () => {
  const message = formatApiError({ apiError: {
    status: 400,
    code: 'json_structure_too_complex',
    requestId,
    retryAfter: null,
    retrySafe: true,
    message: 'request body exceeds the JSON structure complexity limit'
  } })
  assert.equal(message, `请求结构过于复杂，请缩减后重试；请求 ID：${requestId}`)
})

test('formats invalid query parameters consistently', () => {
  const message = formatApiError({ apiError: {
    status: 400,
    code: 'invalid_query_parameter',
    requestId,
    retryAfter: null,
    retrySafe: true,
    message: 'target_word_count must be provided at most once'
  } })
  assert.equal(message, `查询参数无效，请刷新页面后重试；请求 ID：${requestId}`)
})

test('formats unexpected read request bodies consistently', () => {
  const message = formatApiError({ apiError: {
    status: 400,
    code: 'unexpected_request_body',
    requestId,
    retryAfter: null,
    retrySafe: true,
    message: 'GET requests must not include a request body'
  } })
  assert.equal(message, `读取请求不应包含正文，请刷新页面后重试；请求 ID：${requestId}`)
})

test('formats invalid JSON body contracts consistently', () => {
  const message = formatApiError({ apiError: {
    status: 400,
    code: 'invalid_request_body',
    requestId,
    retryAfter: null,
    retrySafe: true,
    message: 'request body does not match the expected JSON contract'
  } })
  assert.equal(message, `请求内容格式无效，请检查后重试；请求 ID：${requestId}`)
})

test('formats unsupported content encodings consistently', () => {
  const message = formatApiError({ apiError: {
    status: 415,
    code: 'unsupported_content_encoding',
    requestId,
    retryAfter: null,
    retrySafe: true,
    message: 'Content-Encoding is not supported'
  } })
  assert.equal(message, `请求正文编码不受支持，请刷新页面后重试；请求 ID：${requestId}`)
})

test('formats invalid UTF-8 request bodies consistently', () => {
  const message = formatApiError({ apiError: {
    status: 400,
    code: 'invalid_utf8_body',
    requestId,
    retryAfter: null,
    retrySafe: true,
    message: 'request body must be valid UTF-8'
  } })
  assert.equal(message, `请求正文不是有效的 UTF-8，请检查后重试；请求 ID：${requestId}`)
})

test('formats repeated security-sensitive headers consistently', () => {
  const message = formatApiError({ apiError: {
    status: 400,
    code: 'ambiguous_request_header',
    requestId,
    retryAfter: null,
    retrySafe: true,
    message: 'request contains a repeated security-sensitive header'
  } })
  assert.equal(message, `请求包含重复的关键请求头，请刷新页面后重试；请求 ID：${requestId}`)
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
