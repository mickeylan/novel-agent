export function withUpdatedAt(data, updatedAt) {
  if (!updatedAt) {
    throw new Error('updated_at is required to prevent overwriting newer data')
  }
  return { ...data, updated_at: updatedAt }
}

export function isUpdateConflict(error) {
  return error?.response?.status === 409 && error?.response?.data?.code === 'update_conflict'
}

export function isSameServerVersion(comparedUpdatedAt, currentUpdatedAt) {
  return Boolean(comparedUpdatedAt && currentUpdatedAt && comparedUpdatedAt === currentUpdatedAt)
}
