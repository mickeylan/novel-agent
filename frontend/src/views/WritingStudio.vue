<template>
  <div class="writing-studio">
    <header class="page-header">
      <h2 class="page-title">AI 写作工作室</h2>
    </header>
    
    <div class="studio-layout">
      <!-- 左侧：章节选择 -->
      <aside class="chapter-selector card">
        <h3>选择章节</h3>
        <div class="chapter-list">
          <div 
            v-for="ch in chapters" 
            :key="ch.id"
            class="chapter-btn"
            :class="{ active: selectedChapter === ch.chapter_number }"
            @click="selectChapter(ch)"
          >
            <span class="ch-num">第{{ ch.chapter_number }}章</span>
            <span class="ch-title">{{ ch.title || '未命名' }}</span>
          </div>
        </div>
      </aside>
      
      <!-- 中间：编辑区 -->
      <main class="editor-area">
        <div v-if="!selectedChapter" class="empty-state">
          <div class="icon">✍️</div>
          <div class="title">选择章节开始写作</div>
          <div class="desc">从左侧选择或创建章节</div>
        </div>
        
        <div v-else class="editor-panel">
          <div class="editor-toolbar">
            <span class="toolbar-title">第{{ selectedChapter }}章</span>
            <div class="toolbar-actions">
              <button class="btn btn-sm btn-secondary" @click="saveContent" :disabled="!hasUnsavedChanges || isSaving">
                {{ isSaving ? '保存中...' : '保存' }}
              </button>
              <button 
                class="btn btn-sm btn-primary" 
                @click="showWriteModal = true"
                :disabled="isGenerating"
              >
                ✨ AI 写作
              </button>
              <button class="btn btn-sm btn-primary" @click="startWorkflow" :disabled="isGenerating">
                ⚙ 创作流水线
              </button>
              <button class="btn btn-sm btn-secondary" @click="validateContent" :disabled="isGenerating || !content">
                ✓ 连贯性检查
              </button>
              <button class="btn btn-sm btn-secondary" @click="previewContext" :disabled="isGenerating">
                ◧ 写作上下文
              </button>
              <button class="btn btn-sm btn-secondary" @click="previewPrompt" :disabled="isGenerating">
                ▣ 完整 Prompt
              </button>
              <button class="btn btn-sm btn-secondary" @click="previewScenePlan" :disabled="isGenerating">
                ▦ 场景拆解
              </button>
              <button class="btn btn-sm btn-secondary" @click="validateSceneCoverage" :disabled="isGenerating || !content">
                ◫ 场景覆盖
              </button>
              <button class="btn btn-sm btn-secondary" @click="extractDataChanges" :disabled="isGenerating || !content">
                ◫ 数据变化
              </button>
              <button class="btn btn-sm btn-secondary" @click="continueChapter" :disabled="isGenerating || !content">
                ＋ 续写
              </button>
              <button class="btn btn-sm btn-secondary" @click="rewriteChapter" :disabled="isGenerating || !content">
                ↺ 按要求重写
              </button>
              <button class="btn btn-sm btn-secondary" @click="summarizeChapter" :disabled="isGenerating || !content">
                ≡ 摘要
              </button>
              <button class="btn btn-sm btn-secondary" @click="analyzeReader" :disabled="isGenerating || !content">
                ◉ 读者分析
              </button>
              <button class="btn btn-sm btn-secondary" @click="analyzeAIGC" :disabled="isGenerating || !content">
                ◌ AIGC 痕迹
              </button>
              <button class="btn btn-sm btn-secondary" @click="analyzeStyle" :disabled="isGenerating || !content">
                ¶ 文风分析
              </button>
              <button class="btn btn-sm btn-secondary" @click="startRevision" :disabled="isGenerating || !content">
                ↻ 修订预览
              </button>
              <button 
                class="btn btn-sm btn-secondary" 
                @click="showPolishModal = true"
                :disabled="isGenerating || !content"
              >
                🎨 AI 润色
              </button>
            </div>
          </div>
          
          <textarea 
            v-model="content"
            class="content-editor"
            @input="markDirty"
            placeholder="在这里输入章节内容..."
          ></textarea>
          
          <div class="editor-status">
            <span>{{ content.length }} 字</span>
            <span v-if="hasUnsavedChanges" class="unsaved-status">未保存</span>
            <span v-else class="saved-status">已保存</span>
            <span v-if="saveError" class="save-error">{{ saveError }}</span>
          </div>
          <div v-if="hasSaveConflict" class="conflict-actions">
            <button class="btn btn-sm btn-secondary" @click="copyLocalDraft">复制本地草稿</button>
            <button class="btn btn-sm btn-secondary" @click="downloadLocalDraft">下载本地草稿</button>
            <button class="btn btn-sm btn-secondary" @click="compareServerVersion">对比服务器版本</button>
            <button class="btn btn-sm btn-secondary" @click="loadServerVersion" :disabled="!serverComparisonUpdatedAt">加载服务器版本</button>
          </div>
        </div>
      </main>
      
      <!-- 右侧：生成面板 -->
      <aside v-if="isGenerating || generationResult" class="generation-panel card">
        <h3>{{ isGenerating ? '生成中...' : '生成结果' }}</h3>
        
        <div v-if="isGenerating" class="generating">
          <div class="spinner"></div>
          <p>正在生成内容，请稍候...</p>
        </div>
        
        <div v-else-if="generationResult" class="result">
          <pre class="result-content">{{ generationResult }}</pre>
          <div class="result-actions">
            <button class="btn btn-sm btn-secondary" @click="previewCandidateDiff">
              查看差异
            </button>
            <button class="btn btn-sm btn-primary" @click="applyResult">
              放入编辑器
            </button>
            <button class="btn btn-sm btn-secondary" @click="generationResult = null">
              关闭
            </button>
          </div>
        </div>
      </aside>

      <aside v-if="serverDiffResult" class="generation-panel card">
        <h3>本地草稿与服务器版本</h3>
        <div class="reader-scores">
          <span>服务器 <b>{{ serverDiffResult.original_runes }}</b></span>
          <span>本地 <b>{{ serverDiffResult.candidate_runes }}</b></span>
          <span>删除 <b>{{ serverDiffResult.removed_runes }}</b></span>
          <span>新增 <b>{{ serverDiffResult.added_runes }}</b></span>
        </div>
        <div class="analysis-issue"><strong>服务器变更区</strong><pre class="result-content">{{ serverDiffResult.prefix_context }}{{ serverDiffResult.original }}{{ serverDiffResult.suffix_context }}</pre></div>
        <div class="analysis-issue"><strong>本地草稿变更区</strong><pre class="result-content">{{ serverDiffResult.prefix_context }}{{ serverDiffResult.candidate }}{{ serverDiffResult.suffix_context }}</pre></div>
        <p class="analysis-summary">只读对比；未更新 Store、正文或本地版本戳。加载前会再次核对服务器版本。{{ serverDiffResult.truncated ? '变更区过长，仅显示前部。' : '' }}</p>
        <button class="btn btn-sm btn-secondary" @click="closeServerComparison">关闭</button>
      </aside>

      <aside v-if="diffResult" class="generation-panel card">
        <h3>候选稿差异</h3>
        <div class="reader-scores">
          <span>原文 <b>{{ diffResult.original_runes }}</b></span>
          <span>候选 <b>{{ diffResult.candidate_runes }}</b></span>
          <span>删除 <b>{{ diffResult.removed_runes }}</b></span>
          <span>新增 <b>{{ diffResult.added_runes }}</b></span>
        </div>
        <div class="analysis-issue"><strong>原变更区</strong><pre class="result-content">{{ diffResult.prefix_context }}{{ diffResult.original }}{{ diffResult.suffix_context }}</pre></div>
        <div class="analysis-issue"><strong>候选变更区</strong><pre class="result-content">{{ diffResult.prefix_context }}{{ diffResult.candidate }}{{ diffResult.suffix_context }}</pre></div>
        <p class="analysis-summary">{{ diffResult.truncated ? '变更区过长，当前仅显示前部。' : '已显示完整变更区。' }} 应用后仍需单独点击保存。</p>
        <button class="btn btn-sm btn-secondary" @click="diffResult = null">关闭</button>
      </aside>

      <aside v-if="sceneCoverageResult" class="generation-panel card">
        <h3>场景覆盖检查</h3>
        <div class="reader-scores">
          <span>覆盖 <b>{{ (sceneCoverageResult.coverage_rate * 100).toFixed(1) }}%</b></span>
          <span>场景 <b>{{ sceneCoverageResult.covered_scenes }}/{{ sceneCoverageResult.total_scenes }}</b></span>
          <span>顺序 <b>{{ sceneCoverageResult.order_valid ? '正常' : '需检查' }}</b></span>
        </div>
        <div v-for="scene in sceneCoverageResult.scenes" :key="scene.scene_index" class="analysis-issue">
          <strong>场景 {{ scene.scene_index }} · {{ scene.covered ? '已覆盖' : '未覆盖' }}</strong>
          <p>{{ scene.purpose }}</p>
          <small>关键词覆盖 {{ (scene.coverage_score * 100).toFixed(0) }}% · 估算 {{ scene.estimated_runes }} 字 / 目标 {{ scene.target_word_count }} 字{{ scene.length_warning ? ' · 篇幅偏差较大' : '' }}</small>
        </div>
        <p class="analysis-summary">纯规则估算，仅供编辑审核，不自动重写或保存。</p>
        <button class="btn btn-sm btn-secondary" @click="sceneCoverageResult = null">关闭</button>
      </aside>

      <aside v-if="scenePlanResult" class="generation-panel card">
        <h3>场景计划</h3>
        <p class="analysis-summary">{{ scenePlanResult.scene_count }} 个场景 · {{ scenePlanResult.target_word_count }} 字 · 来源 {{ scenePlanResult.source }}{{ scenePlanResult.truncated ? '（已截断）' : '' }}</p>
        <div v-for="scene in scenePlanResult.scenes" :key="scene.index" class="analysis-issue">
          <strong>场景 {{ scene.index }} · {{ scene.target_word_count }} 字</strong>
          <p>{{ scene.purpose }}</p>
          <small>切入：{{ scene.entry_hook }}<br />推进：{{ scene.development }}<br />转出：{{ scene.exit_hook }}</small>
        </div>
        <p class="analysis-summary">只读计划，不生成正文、不保存。</p>
        <button class="btn btn-sm btn-secondary" @click="scenePlanResult = null">关闭</button>
      </aside>

      <aside v-if="promptResult" class="generation-panel card">
        <h3>完整 Writer Prompt</h3>
        <p class="analysis-summary">{{ promptResult.prompt_characters }} 字符 · 仅预览，不调用 LLM</p>
        <pre class="result-content">{{ promptResult.prompt }}</pre>
        <button class="btn btn-sm btn-secondary" @click="promptResult = null">关闭</button>
      </aside>

      <aside v-if="contextResult" class="generation-panel card">
        <h3>写作上下文</h3>
        <div class="reader-scores">
          <span>角色 <b>{{ contextResult.character_count }}</b></span>
          <span>前文章节 <b>{{ contextResult.chapter_count }}</b></span>
          <span>截断 <b>{{ contextResult.truncated ? '是' : '否' }}</b></span>
        </div>
        <pre class="result-content">{{ contextResult.prompt_context }}</pre>
        <button class="btn btn-sm btn-secondary" @click="contextResult = null">关闭</button>
      </aside>

      <aside v-if="dataChangesResult" class="generation-panel card">
        <h3>数据变化候选</h3>
        <div class="reader-scores">
          <span>状态变化 <b>{{ dataChangesResult.state_changes?.length || 0 }}</b></span>
          <span>新实体 <b>{{ dataChangesResult.new_entities?.length || 0 }}</b></span>
          <span>伏笔 <b>{{ dataChangesResult.hooks?.length || 0 }}</b></span>
        </div>
        <div v-for="(change, index) in dataChangesResult.state_changes" :key="'state-' + index" class="analysis-issue">
          <strong>{{ change.entity_type }} · {{ change.entity_name }}</strong>
          <p>{{ change.field }}：{{ change.old_value || '未知' }} → {{ change.new_value }}</p>
          <small>证据：{{ change.evidence }} · 置信度 {{ Math.round(change.confidence * 100) }}%</small>
        </div>
        <div v-for="(entity, index) in dataChangesResult.new_entities" :key="'entity-' + index" class="analysis-issue">
          <strong>新 {{ entity.entity_type }} · {{ entity.name }}</strong>
          <p>{{ entity.description }}</p>
          <small>证据：{{ entity.evidence }}</small>
        </div>
        <div v-for="(hook, index) in dataChangesResult.hooks" :key="'hook-' + index" class="analysis-issue">
          <strong>伏笔 · {{ hook.priority }}</strong>
          <p>{{ hook.description }}</p>
          <small>证据：{{ hook.evidence }}</small>
        </div>
        <p class="analysis-summary">仅供审核，不会写入 Story Bible、Truth Files 或账本。</p>
        <button class="btn btn-sm btn-secondary" @click="dataChangesResult = null">关闭</button>
      </aside>

      <aside v-if="workflowResult" class="generation-panel card">
        <h3>创作流水线</h3>
        <div class="reader-scores">
          <span>初稿评分 <b>{{ workflowResult.audit?.overall_score?.toFixed(1) || '-' }}</b></span>
          <span>候选评分 <b>{{ workflowResult.candidate_audit?.overall_score?.toFixed(1) || '-' }}</b></span>
          <span>Critic <b>{{ workflowResult.passed ? '通过' : '待改进' }}</b></span>
          <span>综合建议 <b>{{ workflowResult.quality_gate?.recommended_pass ? '通过' : '需复核' }}</b></span>
          <span>篇幅 <b>{{ workflowResult.candidate_word_count }}/{{ workflowResult.target_word_count }}</b></span>
        </div>
        <p v-if="workflowContext" class="analysis-summary">
          已自动使用 {{ workflowContext.character_count }} 个角色和 {{ workflowContext.chapter_count }} 个前文章节的上下文{{ workflowContext.truncated ? '（已截断）' : '' }}。
        </p>
        <p v-if="workflowRAG" class="analysis-summary">
          RAG：{{ workflowRAG.mode }} · {{ workflowRAG.results?.length || 0 }} 条证据 · 查询「{{ workflowRAG.query }}」
        </p>
        <p v-if="workflowStyle" class="analysis-summary">
          文风约束：参考章节 {{ workflowStyle.reference_chapter_numbers?.join('、') || '无可用样本' }}{{ workflowStyle.reference_truncated ? '（样本已截断）' : '' }}
        </p>
        <p v-if="workflowResult.style_comparison" class="analysis-summary">
          候选稿文风相似度：{{ (workflowResult.style_comparison.similarity * 100).toFixed(1) }}%
        </p>
        <p v-if="workflowScene" class="analysis-summary">
          场景约束：{{ workflowScene.scene_count }} 个场景 · {{ workflowScene.target_word_count }} 字{{ workflowScene.truncated ? '（已截断）' : '' }}
        </p>
        <p v-if="workflowResult.scene_coverage" class="analysis-summary">
          场景覆盖：{{ (workflowResult.scene_coverage.coverage_rate * 100).toFixed(1) }}% · {{ workflowResult.scene_coverage.covered_scenes }}/{{ workflowResult.scene_coverage.total_scenes }} · 顺序{{ workflowResult.scene_coverage.order_valid ? '正常' : '需检查' }}
        </p>
        <p v-if="workflowResult.aigc_analysis" class="analysis-summary">
          AIGC 痕迹：人类化评分 {{ workflowResult.aigc_analysis.overall_score.toFixed(1) }} · 风险 {{ workflowResult.aigc_analysis.risk_level }}
        </p>
        <div v-if="workflowResult.quality_gate" class="analysis-issue">
          <strong>综合质量门（仅建议）</strong>
          <p>{{ workflowResult.quality_gate.passed_checks }}/{{ workflowResult.quality_gate.total_checks }} 项通过</p>
          <small v-for="check in workflowResult.quality_gate.checks" :key="check.name" class="quality-check">
            {{ check.passed ? '✓' : '!' }} {{ check.name }}：{{ check.message }}<br />
          </small>
        </div>
        <div v-for="step in workflowResult.steps" :key="step.name" class="workflow-step">
          <span>{{ step.name }}</span><b :class="step.status">{{ step.status }}</b><small>{{ step.duration_ms }}ms</small>
        </div>
        <button class="btn btn-sm btn-secondary" @click="workflowResult = null">关闭</button>
      </aside>

      <aside v-if="summaryResult" class="generation-panel card">
        <h3>章节摘要</h3>
        <p class="analysis-summary">{{ summaryResult.summary }}</p>
        <div v-if="summaryResult.key_events?.length" class="summary-list">
          <strong>关键事件</strong>
          <ul><li v-for="event in summaryResult.key_events" :key="event">{{ event }}</li></ul>
        </div>
        <div v-if="summaryResult.characters?.length" class="summary-list">
          <strong>涉及角色</strong>
          <p>{{ summaryResult.characters.join('、') }}</p>
        </div>
        <small>{{ summaryResult.emotional_arc }}</small>
        <button class="btn btn-sm btn-secondary" @click="summaryResult = null">关闭</button>
      </aside>

      <aside v-if="styleResult" class="generation-panel card">
        <h3>文风分析</h3>
        <div class="reader-scores">
          <span>平均句长 <b>{{ styleResult.average_sentence_length.toFixed(1) }}</b></span>
          <span>平均段长 <b>{{ styleResult.average_paragraph_length.toFixed(1) }}</b></span>
          <span>对话比例 <b>{{ (styleResult.dialogue_ratio * 100).toFixed(1) }}%</b></span>
          <span>词汇丰富度 <b>{{ (styleResult.vocabulary_richness * 100).toFixed(1) }}%</b></span>
        </div>
        <div v-if="styleComparison?.reference?.character_count" class="analysis-issue">
          <strong>与前文风格对比</strong>
          <p>相似度 {{ (styleComparison.similarity * 100).toFixed(1) }}% · 参考章节 {{ styleComparison.reference_chapter_numbers.join('、') }}</p>
          <small>前文平均句长 {{ styleComparison.reference.average_sentence_length.toFixed(1) }} · 对话比例 {{ (styleComparison.reference.dialogue_ratio * 100).toFixed(1) }}%{{ styleComparison.reference_truncated ? ' · 参考文本已截断' : '' }}</small>
        </div>
        <p v-else-if="styleComparison" class="analysis-summary">当前章节之前没有可用的正文样本。</p>
        <div v-if="styleResult.common_connectives?.length" class="analysis-issue">
          <strong>常用连接词</strong>
          <p>{{ styleResult.common_connectives.map(item => `${item.term}(${item.count})`).join('、') }}</p>
        </div>
        <div class="analysis-issue">
          <strong>标点习惯</strong>
          <p>{{ Object.entries(styleResult.punctuation_style || {}).filter(([, count]) => count > 0).map(([mark, count]) => `${mark} ${count}`).join(' · ') || '无' }}</p>
        </div>
        <p class="analysis-summary">{{ styleResult.sentence_count }} 句 · {{ styleResult.paragraph_count }} 段 · {{ styleResult.character_count }} 字；只读预览，不保存为风格模板。</p>
        <button class="btn btn-sm btn-secondary" @click="styleResult = null">关闭</button>
      </aside>

      <aside v-if="aigcResult" class="generation-panel card">
        <h3>AIGC 痕迹分析</h3>
        <div class="reader-scores">
          <span>人类化评分 <b>{{ aigcResult.overall_score.toFixed(1) }}</b></span>
          <span>风险 <b>{{ aigcResult.risk_level }}</b></span>
          <span>规则 <b>{{ aigcResult.analysis_version }}</b></span>
        </div>
        <div v-for="pattern in aigcResult.patterns_detected" :key="pattern.type + pattern.description" class="analysis-issue">
          <strong>{{ pattern.severity }} · {{ pattern.type }}</strong>
          <p>{{ pattern.description }}（{{ pattern.count }} 次）</p>
        </div>
        <ul class="summary-list"><li v-for="suggestion in aigcResult.suggestions" :key="suggestion">{{ suggestion }}</li></ul>
        <p class="analysis-summary">纯规则预览，仅提供编辑提示，不自动改写正文。</p>
        <button class="btn btn-sm btn-secondary" @click="aigcResult = null">关闭</button>
      </aside>

      <aside v-if="readerResult" class="generation-panel card">
        <h3>读者视角</h3>
        <div class="reader-scores">
          <span>吸引力 <b>{{ readerResult.attractiveness_score }}</b></span>
          <span>钩子 <b>{{ readerResult.hook_strength.toFixed(1) }}</b></span>
          <span>爽点 <b>{{ readerResult.climax_density.toFixed(1) }}</b></span>
        </div>
        <div v-for="(issue, index) in readerResult.issues" :key="index" class="analysis-issue">
          <strong>{{ issue.type }}</strong>
          <p>{{ issue.description }}</p>
          <small>{{ issue.suggestion }}</small>
        </div>
        <button class="btn btn-sm btn-secondary" @click="readerResult = null">关闭</button>
      </aside>

      <aside v-if="analysisResult" class="generation-panel card">
        <h3>连贯性检查</h3>
        <p class="analysis-summary">{{ analysisResult.summary }}</p>
        <div v-for="(issue, index) in analysisResult.issues" :key="index" class="analysis-issue">
          <strong>{{ issue.severity }} · {{ issue.category }}</strong>
          <p>{{ issue.message }}</p>
          <small>{{ issue.suggestion }}</small>
        </div>
        <button class="btn btn-sm btn-secondary" @click="analysisResult = null">关闭</button>
      </aside>
    </div>
    
    <!-- Write Modal -->
    <div v-if="showWriteModal" class="modal-overlay" @click.self="showWriteModal = false">
      <div class="modal card">
        <div class="modal-header">
          <h3>AI 写作</h3>
          <button class="modal-close" @click="showWriteModal = false">×</button>
        </div>
        
        <form @submit.prevent="startWrite" class="modal-body">
          <div class="form-group">
            <label>章节大纲</label>
            <textarea 
              v-model="writeConfig.outline" 
              class="textarea" 
              placeholder="描述本章要讲述的内容..."
              rows="4"
            ></textarea>
          </div>
          
          <div class="form-group checkbox-group">
            <label><input v-model="writeConfig.usePreviousStyle" type="checkbox" /> 使用前文章节文风约束</label>
          </div>

          <div class="form-group checkbox-group">
            <label><input v-model="writeConfig.useScenePlan" type="checkbox" /> 使用场景计划约束</label>
          </div>

          <div class="form-group checkbox-group">
            <label><input v-model="writeConfig.analyzeAIGC" type="checkbox" /> 生成后执行 AIGC 痕迹检查</label>
          </div>

          <div class="form-group checkbox-group">
            <label><input v-model="writeConfig.useRAG" type="checkbox" /> 使用知识库 RAG 证据</label>
          </div>

          <div v-if="writeConfig.useRAG" class="form-group">
            <label>RAG 查询 <span class="text-secondary">（留空则使用章节大纲）</span></label>
            <input v-model="writeConfig.ragQuery" class="input" placeholder="要检索的主题、场景或写法" />
          </div>

          <div class="form-group">
            <label>目标字数</label>
            <input 
              v-model.number="writeConfig.targetWordCount" 
              type="number" 
              class="input" 
              min="1000"
              max="20000"
            />
          </div>
          
          <div class="modal-actions">
            <button type="button" class="btn btn-secondary" @click="showWriteModal = false">
              取消
            </button>
            <button type="submit" class="btn btn-primary">
              开始生成
            </button>
          </div>
        </form>
      </div>
    </div>

    <!-- Polish Modal -->
    <div v-if="showPolishModal" class="modal-overlay" @click.self="showPolishModal = false">
      <div class="modal card">
        <div class="modal-header">
          <h3>AI 润色</h3>
          <button class="modal-close" @click="showPolishModal = false">×</button>
        </div>
        <form @submit.prevent="startPolish" class="modal-body">
          <div class="form-group">
            <label>风格要求</label>
            <textarea
              v-model="polishStyleGuide"
              class="textarea"
              placeholder="例如：语言凝练、增强画面感、保持人物语气一致"
              rows="4"
            ></textarea>
          </div>
          <div class="modal-actions">
            <button type="button" class="btn btn-secondary" @click="showPolishModal = false">取消</button>
            <button type="submit" class="btn btn-primary">开始润色</button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { onBeforeRouteLeave, useRoute } from 'vue-router'
import { useChapterStore } from '../stores/chapter'
import { creationApi } from '../api/creation'
import { chapterApi } from '../api/chapter'
import { isSameServerVersion } from '../api/concurrency'
import { buildDraftFilename, buildDraftText } from '../utils/draftExport'

const route = useRoute()
const chapterStore = useChapterStore()

const projectId = computed(() => route.params.id)
const chapters = computed(() => chapterStore.chapters)

const selectedChapter = ref(null)
const content = ref('')
const savedContent = ref('')
const hasUnsavedChanges = ref(false)
const isSaving = ref(false)
const saveError = ref('')
const hasSaveConflict = ref(false)
const isGenerating = ref(false)
const generationResult = ref(null)
const diffResult = ref(null)
const serverDiffResult = ref(null)
const serverComparisonUpdatedAt = ref('')
const showWriteModal = ref(false)
const showPolishModal = ref(false)
const polishStyleGuide = ref('')
const analysisResult = ref(null)
const readerResult = ref(null)
const aigcResult = ref(null)
const styleResult = ref(null)
const styleComparison = ref(null)
const summaryResult = ref(null)
const workflowResult = ref(null)
const workflowContext = ref(null)
const workflowRAG = ref(null)
const workflowStyle = ref(null)
const workflowScene = ref(null)
const dataChangesResult = ref(null)
const contextResult = ref(null)
const promptResult = ref(null)
const scenePlanResult = ref(null)
const sceneCoverageResult = ref(null)

const writeConfig = ref({
  outline: '',
  targetWordCount: 5000,
  useRAG: false,
  ragQuery: '',
  usePreviousStyle: false,
  useScenePlan: false,
  analyzeAIGC: true
})

function warnBeforeUnload(event) {
  if (!hasUnsavedChanges.value) return
  event.preventDefault()
  event.returnValue = ''
}

onMounted(() => {
  chapterStore.fetchChapters(projectId.value)
  window.addEventListener('beforeunload', warnBeforeUnload)
})

onBeforeUnmount(() => {
  window.removeEventListener('beforeunload', warnBeforeUnload)
})

function selectChapter(ch) {
  if (ch.chapter_number === selectedChapter.value) return
  if (hasUnsavedChanges.value && !window.confirm('当前章节有未保存修改，切换章节将丢失这些修改。确定继续吗？')) return
  selectedChapter.value = ch.chapter_number
  savedContent.value = ch.content || ''
  content.value = savedContent.value
  hasUnsavedChanges.value = false
  saveError.value = ''
  hasSaveConflict.value = false
  generationResult.value = null
  diffResult.value = null
  serverDiffResult.value = null
  serverComparisonUpdatedAt.value = ''
}

function markDirty() {
  hasUnsavedChanges.value = content.value !== savedContent.value
  if (!hasSaveConflict.value) saveError.value = ''
  serverDiffResult.value = null
  serverComparisonUpdatedAt.value = ''
}

async function saveContent() {
  if (!selectedChapter.value || !hasUnsavedChanges.value || isSaving.value) return
  isSaving.value = true
  saveError.value = ''
  hasSaveConflict.value = false
  serverDiffResult.value = null
  serverComparisonUpdatedAt.value = ''
  const contentToSave = content.value
  try {
    await chapterStore.updateChapter(projectId.value, selectedChapter.value, { content: contentToSave })
    savedContent.value = contentToSave
    hasUnsavedChanges.value = content.value !== contentToSave
  } catch (error) {
    hasSaveConflict.value = error?.response?.status === 409 && error?.response?.data?.code === 'update_conflict'
    saveError.value = hasSaveConflict.value
      ? '保存冲突：服务器内容已更新。本地草稿已保留。'
      : '保存失败，当前草稿已保留。'
    console.error('Failed to save chapter:', error)
  } finally {
    isSaving.value = false
  }
}

async function copyLocalDraft() {
  try {
    await navigator.clipboard.writeText(content.value)
    saveError.value = '本地草稿已复制到剪贴板。'
  } catch (error) {
    saveError.value = '复制失败，请手动全选编辑器内容复制。'
    console.error('Failed to copy local draft:', error)
  }
}

function downloadLocalDraft() {
  const selected = chapters.value.find(chapter => chapter.chapter_number === selectedChapter.value)
  const text = buildDraftText(selectedChapter.value, content.value, selected?.updated_at)
  const url = URL.createObjectURL(new Blob([text], { type: 'text/plain;charset=utf-8' }))
  const link = document.createElement('a')
  link.href = url
  link.download = buildDraftFilename(selectedChapter.value)
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
  saveError.value = '本地草稿已下载；编辑器内容和版本状态未改变。'
}

function closeServerComparison() {
  serverDiffResult.value = null
  serverComparisonUpdatedAt.value = ''
}

async function compareServerVersion() {
  if (!hasSaveConflict.value || !selectedChapter.value) return
  closeServerComparison()
  try {
    const result = await creationApi.diffPreview(projectId.value, selectedChapter.value, {
      candidate: content.value,
      max_changed_runes: 4000,
      context_runes: 120
    })
    serverDiffResult.value = result.diff
    serverComparisonUpdatedAt.value = result.updated_at
  } catch (error) {
    closeServerComparison()
    saveError.value = '读取服务器版本进行对比失败，本地草稿仍保留。'
    console.error('Failed to compare server version:', error)
  }
}

async function loadServerVersion() {
  if (!hasSaveConflict.value || !selectedChapter.value || !serverComparisonUpdatedAt.value) return
  if (!window.confirm('加载已对比的服务器版本将丢弃编辑器中的本地草稿。建议先复制草稿。确定继续吗？')) return
  try {
    const chapter = await chapterApi.get(projectId.value, selectedChapter.value)
    if (!isSameServerVersion(serverComparisonUpdatedAt.value, chapter.updated_at)) {
      serverDiffResult.value = null
      serverComparisonUpdatedAt.value = ''
      saveError.value = '服务器版本在对比后再次变化，请重新对比后再决定是否加载。'
      return
    }
    chapterStore.acceptChapter(chapter)
    savedContent.value = chapter.content || ''
    content.value = savedContent.value
    hasUnsavedChanges.value = false
    hasSaveConflict.value = false
    saveError.value = ''
    generationResult.value = null
    diffResult.value = null
    serverDiffResult.value = null
    serverComparisonUpdatedAt.value = ''
  } catch (error) {
    saveError.value = '加载服务器版本失败，本地草稿仍保留。'
    console.error('Failed to load server version:', error)
  }
}

async function startWrite() {
  if (!selectedChapter.value) return
  
  showWriteModal.value = false
  isGenerating.value = true
  generationResult.value = null
  
  try {
    const result = await creationApi.writeChapter(projectId.value, selectedChapter.value, {
      outline: writeConfig.value.outline,
      target_word_count: writeConfig.value.targetWordCount
    })
    
    generationResult.value = result.content
  } catch (error) {
    console.error('Failed to write chapter:', error)
  } finally {
    isGenerating.value = false
  }
}

async function startWorkflow() {
  if (!selectedChapter.value) return
  const chapter = chapters.value.find(item => item.chapter_number === selectedChapter.value)
  const outline = writeConfig.value.outline || chapter?.outline || ''
  if (!outline.trim()) {
    window.alert('请先填写或保存章节大纲')
    return
  }
  isGenerating.value = true
  generationResult.value = null
  workflowResult.value = null
  workflowContext.value = null
  workflowRAG.value = null
  workflowStyle.value = null
  workflowScene.value = null
  try {
    const result = await creationApi.workflowPreview(projectId.value, selectedChapter.value, {
      outline,
      target_word_count: writeConfig.value.targetWordCount,
      auto_revise: true,
      revision_threshold: 6.0,
      extract_data_changes: true,
      analyze_aigc: writeConfig.value.analyzeAIGC,
      use_rag: writeConfig.value.useRAG,
      rag_query: writeConfig.value.ragQuery,
      rag_mode: 'hybrid',
      rag_limit: 5,
      use_previous_style: writeConfig.value.usePreviousStyle,
      style_samples: 5,
      use_scene_plan: writeConfig.value.useScenePlan,
      max_scenes: 8
    })
    workflowResult.value = result.workflow
    workflowContext.value = result.writing_context
    workflowRAG.value = result.rag_context
    workflowStyle.value = result.style_context
    workflowScene.value = result.scene_context
    dataChangesResult.value = result.workflow.data_changes || null
    generationResult.value = result.workflow.candidate
  } catch (error) {
    console.error('Failed to run chapter workflow:', error)
  } finally {
    isGenerating.value = false
  }
}

async function validateSceneCoverage() {
  if (!selectedChapter.value || !content.value.trim()) return
  const chapter = chapters.value.find(item => item.chapter_number === selectedChapter.value)
  const outline = writeConfig.value.outline || chapter?.outline || ''
  if (!outline.trim() && !(chapter?.key_events?.length)) {
    window.alert('请先填写章节大纲或关键事件')
    return
  }
  isGenerating.value = true
  sceneCoverageResult.value = null
  try {
    const result = await creationApi.sceneCoverage(projectId.value, selectedChapter.value, {
      content: content.value,
      outline,
      key_events: chapter?.key_events || [],
      target_word_count: writeConfig.value.targetWordCount,
      max_scenes: 8
    })
    sceneCoverageResult.value = result.coverage
  } catch (error) {
    console.error('Failed to validate scene coverage:', error)
  } finally {
    isGenerating.value = false
  }
}

async function previewScenePlan() {
  if (!selectedChapter.value) return
  const chapter = chapters.value.find(item => item.chapter_number === selectedChapter.value)
  const outline = writeConfig.value.outline || chapter?.outline || ''
  if (!outline.trim() && !(chapter?.key_events?.length)) {
    window.alert('请先填写章节大纲或关键事件')
    return
  }
  isGenerating.value = true
  scenePlanResult.value = null
  try {
    const result = await creationApi.scenePlanPreview(projectId.value, selectedChapter.value, {
      outline,
      key_events: chapter?.key_events || [],
      target_word_count: writeConfig.value.targetWordCount,
      max_scenes: 8
    })
    scenePlanResult.value = result.plan
  } catch (error) {
    console.error('Failed to preview scene plan:', error)
  } finally {
    isGenerating.value = false
  }
}

async function previewPrompt() {
  if (!selectedChapter.value) return
  const chapter = chapters.value.find(item => item.chapter_number === selectedChapter.value)
  const outline = writeConfig.value.outline || chapter?.outline || ''
  if (!outline.trim()) {
    window.alert('请先填写或保存章节大纲')
    return
  }
  isGenerating.value = true
  promptResult.value = null
  try {
    promptResult.value = await creationApi.promptPreview(projectId.value, selectedChapter.value, {
      outline,
      target_word_count: writeConfig.value.targetWordCount,
      use_context: true,
      use_previous_style: writeConfig.value.usePreviousStyle,
      style_samples: 5,
      use_scene_plan: writeConfig.value.useScenePlan,
      max_scenes: 8,
      use_rag: writeConfig.value.useRAG,
      rag_query: writeConfig.value.ragQuery,
      rag_mode: 'hybrid',
      rag_limit: 5
    })
  } catch (error) {
    console.error('Failed to preview writer prompt:', error)
  } finally {
    isGenerating.value = false
  }
}

async function previewContext() {
  if (!selectedChapter.value) return
  isGenerating.value = true
  contextResult.value = null
  try {
    const result = await creationApi.writingContextPreview(projectId.value, selectedChapter.value)
    contextResult.value = result.context
  } catch (error) {
    console.error('Failed to build writing context:', error)
  } finally {
    isGenerating.value = false
  }
}

async function extractDataChanges() {
  if (!selectedChapter.value || !content.value.trim()) return
  isGenerating.value = true
  dataChangesResult.value = null
  try {
    const result = await creationApi.dataChangesPreview(projectId.value, selectedChapter.value, {
      content: content.value
    })
    dataChangesResult.value = result.changes
  } catch (error) {
    console.error('Failed to extract data changes:', error)
  } finally {
    isGenerating.value = false
  }
}

async function continueChapter() {
  if (!selectedChapter.value) return
  const instructions = window.prompt('请输入续写要求，例如：延续当前冲突并写到角色抵达目的地')
  if (!instructions?.trim()) return
  isGenerating.value = true
  generationResult.value = null
  try {
    const result = await creationApi.continueChapterPreview(projectId.value, selectedChapter.value, {
      instructions: instructions.trim(),
      target_word_count: 2000
    })
    generationResult.value = result.candidate
  } catch (error) {
    console.error('Failed to continue chapter:', error)
  } finally {
    isGenerating.value = false
  }
}

async function rewriteChapter() {
  if (!selectedChapter.value) return
  const instructions = window.prompt('请输入重写要求，例如：加强冲突、减少说明性文字、保留核心剧情')
  if (!instructions?.trim()) return
  isGenerating.value = true
  generationResult.value = null
  try {
    const result = await creationApi.rewriteChapterPreview(projectId.value, selectedChapter.value, {
      instructions: instructions.trim()
    })
    generationResult.value = result.candidate
  } catch (error) {
    console.error('Failed to rewrite chapter:', error)
  } finally {
    isGenerating.value = false
  }
}

async function summarizeChapter() {
  if (!selectedChapter.value) return
  isGenerating.value = true
  summaryResult.value = null
  try {
    const result = await creationApi.summarizeChapterPreview(projectId.value, selectedChapter.value)
    summaryResult.value = result.summary
  } catch (error) {
    console.error('Failed to summarize chapter:', error)
  } finally {
    isGenerating.value = false
  }
}

async function validateContent() {
  if (!selectedChapter.value) return
  isGenerating.value = true
  generationResult.value = null
  try {
    analysisResult.value = await creationApi.validateChapter(projectId.value, selectedChapter.value)
  } catch (error) {
    console.error('Failed to validate chapter:', error)
  } finally {
    isGenerating.value = false
  }
}

async function analyzeStyle() {
  if (!selectedChapter.value || !content.value.trim()) return
  isGenerating.value = true
  styleResult.value = null
  styleComparison.value = null
  try {
    const result = await creationApi.styleAnalysis(projectId.value, selectedChapter.value, {
      content: content.value,
      compare_previous: true,
      sample_chapters: 5
    })
    styleResult.value = result.profile
    styleComparison.value = result.comparison || null
  } catch (error) {
    console.error('Failed to analyze writing style:', error)
  } finally {
    isGenerating.value = false
  }
}

async function analyzeAIGC() {
  if (!selectedChapter.value || !content.value.trim()) return
  isGenerating.value = true
  aigcResult.value = null
  try {
    const result = await creationApi.aigcAnalysis(projectId.value, selectedChapter.value, { content: content.value })
    aigcResult.value = result.analysis
  } catch (error) {
    console.error('Failed to analyze AIGC patterns:', error)
  } finally {
    isGenerating.value = false
  }
}

async function analyzeReader() {
  if (!selectedChapter.value) return
  isGenerating.value = true
  generationResult.value = null
  analysisResult.value = null
  try {
    const result = await creationApi.readerAnalysis(projectId.value, selectedChapter.value)
    readerResult.value = result.analysis
  } catch (error) {
    console.error('Failed to analyze reader perspective:', error)
  } finally {
    isGenerating.value = false
  }
}

async function startRevision() {
  if (!selectedChapter.value) return
  isGenerating.value = true
  generationResult.value = null
  analysisResult.value = null
  try {
    const result = await creationApi.reviseChapterPreview(projectId.value, selectedChapter.value)
    generationResult.value = result.revision.candidate
  } catch (error) {
    console.error('Failed to revise chapter:', error)
  } finally {
    isGenerating.value = false
  }
}

async function startPolish() {
  if (!selectedChapter.value) return

  showPolishModal.value = false
  isGenerating.value = true
  generationResult.value = null
  try {
    const result = await creationApi.polishChapter(projectId.value, selectedChapter.value, {
      style_guide: polishStyleGuide.value
    })
    generationResult.value = result.content
  } catch (error) {
    console.error('Failed to polish chapter:', error)
  } finally {
    isGenerating.value = false
  }
}

async function previewCandidateDiff() {
  if (!selectedChapter.value || generationResult.value == null) return
  diffResult.value = null
  try {
    const result = await creationApi.diffPreview(projectId.value, selectedChapter.value, {
      original: content.value,
      candidate: generationResult.value,
      max_changed_runes: 4000,
      context_runes: 120
    })
    diffResult.value = result.diff
  } catch (error) {
    console.error('Failed to preview candidate diff:', error)
  }
}

function applyResult() {
  if (generationResult.value == null) return
  if (!window.confirm('候选稿将替换编辑器中的当前草稿，但不会自动保存。确定继续吗？')) return
  content.value = generationResult.value
  generationResult.value = null
  diffResult.value = null
  markDirty()
}

onBeforeRouteLeave(() => {
  if (!hasUnsavedChanges.value) return true
  return window.confirm('当前章节有未保存修改，离开页面将丢失这些修改。确定离开吗？')
})
</script>

<style scoped>
.writing-studio {
  height: calc(100vh - 120px);
  display: flex;
  flex-direction: column;
}

.page-header {
  margin-bottom: 16px;
}

.page-title {
  font-size: 20px;
  font-weight: 600;
}

.studio-layout {
  display: grid;
  grid-template-columns: 200px 1fr 300px;
  gap: 16px;
  flex: 1;
  min-height: 0;
}

.chapter-selector {
  padding: 16px;
  overflow-y: auto;
}

.chapter-selector h3 {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-secondary);
  margin-bottom: 12px;
  text-transform: uppercase;
}

.chapter-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.chapter-btn {
  padding: 10px 12px;
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: var(--transition);
}

.chapter-btn:hover {
  background: var(--bg-tertiary);
}

.chapter-btn.active {
  background: var(--accent-soft);
}

.ch-num {
  display: block;
  font-size: 12px;
  font-weight: 600;
  color: var(--accent);
  margin-bottom: 2px;
}

.ch-title {
  display: block;
  font-size: 13px;
  color: var(--text-secondary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.editor-area {
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.editor-panel {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.editor-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 16px;
  background: var(--bg-secondary);
  border-radius: var(--radius-md) var(--radius-md) 0 0;
  border: 1px solid var(--border);
  border-bottom: none;
}

.toolbar-title {
  font-weight: 600;
}

.toolbar-actions {
  display: flex;
  gap: 8px;
}

.content-editor {
  flex: 1;
  padding: 16px;
  background: var(--card);
  border: 1px solid var(--border);
  border-radius: 0 0 var(--radius-md) var(--radius-md);
  color: var(--text);
  font-size: 15px;
  line-height: 1.8;
  font-family: 'Georgia', serif;
  resize: none;
}

.content-editor:focus {
  outline: none;
}

.editor-status {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 8px 16px;
  font-size: 12px;
  color: var(--text-tertiary);
  background: var(--bg-secondary);
  border: 1px solid var(--border);
  border-top: none;
  border-radius: 0 0 var(--radius-md) var(--radius-md);
}

.unsaved-status { color: #d97706; font-weight: 600; }
.saved-status { color: #15803d; }
.save-error { color: #dc2626; }

.conflict-actions {
  display: flex;
  gap: 8px;
  padding: 8px 16px;
  border: 1px solid #fecaca;
  border-top: none;
  background: #fef2f2;
}

.generation-panel {
  padding: 16px;
  overflow-y: auto;
}

.generation-panel h3 {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-secondary);
  margin-bottom: 16px;
  text-transform: uppercase;
}

.generating {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  padding: 40px 20px;
  color: var(--text-secondary);
}

.result-content {
  font-size: 13px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-word;
  max-height: 400px;
  overflow-y: auto;
  padding: 12px;
  background: var(--bg-secondary);
  border-radius: var(--radius-sm);
  margin-bottom: 16px;
}

.result-actions {
  display: flex;
  gap: 8px;
}

.analysis-summary { margin-bottom: 14px; color: var(--text-secondary); font-size: 13px; }
.reader-scores { display: grid; grid-template-columns: repeat(3, 1fr); gap: 6px; margin-bottom: 14px; }
.reader-scores span { padding: 8px; border-radius: var(--radius-sm); background: var(--bg-secondary); color: var(--text-secondary); font-size: 11px; text-align: center; }
.reader-scores b { display: block; margin-top: 3px; color: var(--accent); font-size: 18px; }
.analysis-issue { padding: 10px 0; border-top: 1px solid var(--border); }
.analysis-issue strong { color: var(--orange); font-size: 12px; text-transform: uppercase; }
.analysis-issue p { margin: 5px 0; font-size: 13px; }
.analysis-issue small { color: var(--text-secondary); line-height: 1.5; }
.summary-list { margin: 12px 0; font-size: 13px; color: var(--text-secondary); }
.summary-list strong { display: block; margin-bottom: 5px; color: var(--text); }
.summary-list ul { padding-left: 18px; }
.workflow-step { display: grid; grid-template-columns: 1fr auto auto; gap: 8px; align-items: center; padding: 7px 0; border-top: 1px solid var(--border); font-size: 12px; }
.workflow-step b { color: var(--green); }
.workflow-step b.failed { color: var(--red); }
.workflow-step small { color: var(--text-tertiary); }

/* Modal */
.modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: rgba(0, 0, 0, 0.7);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
  padding: 20px;
}

.modal {
  width: 100%;
  max-width: 480px;
}

.modal-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 20px 24px;
  border-bottom: 1px solid var(--border);
}

.modal-header h3 {
  font-size: 18px;
  font-weight: 600;
}

.modal-close {
  width: 32px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: none;
  border: none;
  font-size: 24px;
  color: var(--text-secondary);
  cursor: pointer;
}

.modal-body {
  padding: 24px;
}

.form-group {
  margin-bottom: 20px;
}

.form-group label {
  display: block;
  font-size: 14px;
  font-weight: 500;
  margin-bottom: 8px;
  color: var(--text-secondary);
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  padding-top: 16px;
  border-top: 1px solid var(--border);
}
</style>
