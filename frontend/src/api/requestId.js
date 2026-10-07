const REQUEST_ID_HEADER = 'X-Request-ID'
const SAFE_REQUEST_ID = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i

function existingRequestID(headers) {
  if (!headers) return ''
  if (typeof headers.get === 'function') {
    const value = headers.get(REQUEST_ID_HEADER)
    return typeof value === 'string' && SAFE_REQUEST_ID.test(value) ? value : ''
  }
  const key = Object.keys(headers).find(name => name.toLowerCase() === REQUEST_ID_HEADER.toLowerCase())
  const value = key ? headers[key] : ''
  return typeof value === 'string' && SAFE_REQUEST_ID.test(value) ? value : ''
}

export function attachRequestID(config, uuidFactory = () => globalThis.crypto?.randomUUID?.()) {
  const headers = config.headers || {}
  if (existingRequestID(headers)) return config
  const requestId = uuidFactory()
  if (typeof requestId !== 'string' || !SAFE_REQUEST_ID.test(requestId)) return config
  if (typeof headers.set === 'function') {
    headers.set(REQUEST_ID_HEADER, requestId)
  } else {
    const existingKey = Object.keys(headers).find(name => name.toLowerCase() === REQUEST_ID_HEADER.toLowerCase())
    if (existingKey && existingKey !== REQUEST_ID_HEADER) delete headers[existingKey]
    headers[REQUEST_ID_HEADER] = requestId
  }
  config.headers = headers
  return config
}
