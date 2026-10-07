export const STANDARD_API_TIMEOUT_MS = 30_000
export const AI_PREVIEW_TIMEOUT_MS = 20 * 60_000

export function withTimeout(timeout) {
  if (!Number.isInteger(timeout) || timeout < 1 || timeout > AI_PREVIEW_TIMEOUT_MS) {
    throw new RangeError(`timeout must be between 1 and ${AI_PREVIEW_TIMEOUT_MS} milliseconds`)
  }
  return { timeout }
}

export const aiPreviewRequest = Object.freeze(withTimeout(AI_PREVIEW_TIMEOUT_MS))
