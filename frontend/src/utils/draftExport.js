export function buildDraftFilename(chapterNumber, now = new Date()) {
  const chapter = Number.isInteger(Number(chapterNumber)) && Number(chapterNumber) > 0
    ? `chapter-${Number(chapterNumber)}`
    : 'chapter-unknown'
  const timestamp = now.toISOString().replace(/[:.]/g, '-').replace('T', '_').replace('Z', '')
  return `${chapter}_local-draft_${timestamp}.txt`
}

export function buildDraftText(chapterNumber, content, savedUpdatedAt = '') {
  const lines = [
    `章节：第 ${chapterNumber || '未知'} 章`,
    `导出时间：${new Date().toISOString()}`,
    `本地读取版本：${savedUpdatedAt || '未知'}`,
    '',
    String(content ?? '')
  ]
  return `\uFEFF${lines.join('\n')}`
}
