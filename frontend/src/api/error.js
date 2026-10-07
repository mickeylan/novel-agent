const SAFE_CODE = /^[a-z0-9_]{1,64}$/
const SAFE_REQUEST_ID = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i

export function normalizeApiError(error) {
  const response = error?.response
  const data = response?.data
  const code = typeof data?.code === 'string' && SAFE_CODE.test(data.code) ? data.code : 'request_failed'
  const headerRequestId = response?.headers?.['x-request-id']
  const requestHeaders = error?.config?.headers
  const sentRequestId = typeof requestHeaders?.get === 'function'
    ? requestHeaders.get('X-Request-ID')
    : requestHeaders?.['X-Request-ID'] || requestHeaders?.['x-request-id']
  const bodyRequestId = data?.request_id || data?.error_id
  const requestId = [headerRequestId, bodyRequestId, sentRequestId].find(value => typeof value === 'string' && SAFE_REQUEST_ID.test(value)) || ''
  const retryAfterRaw = response?.headers?.['retry-after']
  const retryAfter = typeof retryAfterRaw === 'string' && /^\d{1,5}$/.test(retryAfterRaw) ? Number(retryAfterRaw) : null
  const serverMessage = typeof data?.error === 'string' && data.error.length <= 300 ? data.error : ''

  return {
    status: Number.isInteger(response?.status) ? response.status : 0,
    code,
    requestId,
    retryAfter,
    retrySafe: data?.retry_safe !== false,
    message: serverMessage || (error?.code === 'ECONNABORTED' ? '请求超时，服务端可能仍在处理' : error?.message === 'Network Error' ? '网络连接失败' : '请求失败')
  }
}

export function formatApiError(error, fallback = '请求失败') {
  const detail = error?.apiError || normalizeApiError(error)
  let message = detail.message && detail.message !== 'internal server error' ? detail.message : fallback
  if (detail.code === 'ai_capacity_exceeded') {
    message = `AI 服务繁忙，请稍后重试${detail.retryAfter === null ? '' : `（建议等待 ${detail.retryAfter} 秒）`}`
  } else if (detail.code === 'update_conflict') {
    message = '服务器内容已更新，请保留本地草稿并进行版本对比'
  } else if (detail.code === 'unsupported_media_type') {
    message = '请求格式不受支持，请刷新页面后重试'
  } else if (detail.code === 'write_result_unavailable' || detail.retrySafe === false) {
    message = '写入可能已成功，但无法读取保存结果。请先刷新核对服务器数据，不要直接重试'
  } else if (detail.code === 'invalid_chapter_number') {
    message = '章节号无效，必须是 1–1000000 之间的整数'
  } else if (detail.code === 'invalid_project_id') {
    message = '项目 ID 格式无效'
  } else if (detail.code === 'invalid_character_id') {
    message = '角色 ID 格式无效'
  } else if (detail.code === 'duplicate_json_key') {
    message = '请求包含重复字段，请刷新页面后重试'
  } else if (detail.code === 'json_nesting_too_deep') {
    message = '请求结构嵌套过深，请简化后重试'
  }
  if (detail.requestId) message += `；请求 ID：${detail.requestId}`
  return message
}
