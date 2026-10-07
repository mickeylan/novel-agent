import { api } from './index'
import { aiPreviewRequest } from './timeouts'

export const creationApi = {
  // 生成大纲（预览，不自动保存）
  generateOutline: (projectId, data) => api.post(`/projects/${projectId}/generate-outline`, data, aiPreviewRequest),
  generateChapterOutlines: (projectId, data) => api.post(`/projects/${projectId}/chapters/generate-outlines`, data, aiPreviewRequest),
  
  // AI 写作预览
  writeChapter: (projectId, chapterNum, data) => api.post(`/projects/${projectId}/chapters/${chapterNum}/write`, data, aiPreviewRequest),
  workflowBudgetPreview: (data) => api.post('/workflow-budget-preview', data),
  workflowPreview: (projectId, chapterNum, data) => api.post(`/projects/${projectId}/chapters/${chapterNum}/workflow-preview`, data, aiPreviewRequest),
  continueChapterPreview: (projectId, chapterNum, data) => api.post(`/projects/${projectId}/chapters/${chapterNum}/continue-preview`, data, aiPreviewRequest),
  rewriteChapterPreview: (projectId, chapterNum, data) => api.post(`/projects/${projectId}/chapters/${chapterNum}/rewrite-preview`, data, aiPreviewRequest),
  summarizeChapterPreview: (projectId, chapterNum) => api.get(`/projects/${projectId}/chapters/${chapterNum}/summary-preview`, aiPreviewRequest),
  
  // 章节 Prompt
  getChapterPrompt: (projectId, chapterNum, targetWordCount = 5000) => api.get(`/projects/${projectId}/chapters/${chapterNum}/prompt`, { params: { target_word_count: targetWordCount } }),
  promptPreview: (projectId, chapterNum, data) => api.post(`/projects/${projectId}/chapters/${chapterNum}/prompt-preview`, data, aiPreviewRequest),
  scenePlanPreview: (projectId, chapterNum, data) => api.post(`/projects/${projectId}/chapters/${chapterNum}/scene-plan-preview`, data),
  writingContextPreview: (projectId, chapterNum, maxRunes = 8000) => api.get(`/projects/${projectId}/chapters/${chapterNum}/context-preview`, { params: { max_runes: maxRunes } }),

  // AI 润色（预览，不自动保存）
  polishChapter: (projectId, chapterNum, data) => api.post(`/projects/${projectId}/chapters/${chapterNum}/polish`, data, aiPreviewRequest),
  
  // AI 评估
  auditChapter: (projectId, data) => api.post(`/projects/${projectId}/audit`, data, aiPreviewRequest),
  evaluateChapter: (projectId, data) => api.post(`/projects/${projectId}/evaluate`, data, aiPreviewRequest),

  // 连贯性与修订预览（均不自动保存）
  validateChapter: (projectId, chapterNum, data = {}) => api.post(`/projects/${projectId}/chapters/${chapterNum}/validate`, data),
  diffPreview: (projectId, chapterNum, data) => api.post(`/projects/${projectId}/chapters/${chapterNum}/diff-preview`, data),
  sceneCoverage: (projectId, chapterNum, data) => api.post(`/projects/${projectId}/chapters/${chapterNum}/scene-coverage`, data),
  aigcAnalysis: (projectId, chapterNum, data = {}) => api.post(`/projects/${projectId}/chapters/${chapterNum}/aigc-analysis`, data),
  styleAnalysis: (projectId, chapterNum, data = {}) => api.post(`/projects/${projectId}/chapters/${chapterNum}/style-analysis`, data),
  dataChangesPreview: (projectId, chapterNum, data = {}) => api.post(`/projects/${projectId}/chapters/${chapterNum}/data-changes-preview`, data, aiPreviewRequest),
  readerAnalysis: (projectId, chapterNum) => api.get(`/projects/${projectId}/chapters/${chapterNum}/reader-analysis`),
  reviseChapterPreview: (projectId, chapterNum) => api.post(`/projects/${projectId}/chapters/${chapterNum}/revise-preview`, {}, aiPreviewRequest)
}

export default creationApi
