package agents

import (
	"context"
	"fmt"
	"strings"

	"novel-agent/internal/service/llm"
)

// Writer Agent - Chapter content generation
type Writer struct {
	client *llm.Client
}

func NewWriter(client *llm.Client) *Writer {
	return &Writer{client: client}
}

// WriteChapter writes a chapter based on outline and context
func (w *Writer) WriteChapter(ctx context.Context, outline, styleGuide, memoryContext, previousChapter string, targetWordCount int) (string, error) {
	prompt := w.BuildWritingPrompt(outline, styleGuide, memoryContext, previousChapter, targetWordCount)

	// Use streaming for long generations
	content, err := w.client.Generate(ctx, prompt,
		llm.WithTemperature(0.5),             // Lower temperature for more focused output
		llm.WithMaxTokens(targetWordCount*2), // Allow room for Chinese characters
		llm.WithStream(true),
	)
	if err != nil {
		return "", fmt.Errorf("failed to write chapter: %w", err)
	}

	// Post-process to clean up.
	content = cleanChapterContent(content)
	if err := validateGeneratedContent(content); err != nil {
		return "", fmt.Errorf("invalid generated chapter: %w", err)
	}

	return content, nil
}

// BuildWritingPrompt returns the exact prompt used for chapter generation without invoking the LLM.
func (w *Writer) BuildWritingPrompt(outline, styleGuide, memoryContext, previousChapter string, targetWordCount int) string {
	var sb strings.Builder

	sb.WriteString("你是一位专业的小说作家。请根据以下大纲和上下文，创作一章精彩的小说内容。\n\n")

	// Style guide
	if styleGuide != "" {
		sb.WriteString("【写作风格指南】\n")
		sb.WriteString(styleGuide)
		sb.WriteString("\n\n")
	}

	// Memory context
	if memoryContext != "" {
		sb.WriteString("【记忆上下文】\n")
		sb.WriteString(memoryContext)
		sb.WriteString("\n\n")
	}

	// Previous chapter
	if previousChapter != "" {
		sb.WriteString("【前章结尾】\n")
		sb.WriteString(previousChapter)
		sb.WriteString("\n\n")
	}

	// Chapter outline
	sb.WriteString("【本章大纲】\n")
	sb.WriteString(outline)
	sb.WriteString("\n\n")

	// Writing instructions
	sb.WriteString(fmt.Sprintf(`【写作要求】
1. 字数目标：约 %d 字
2. 使用简体中文写作
3. 遵循大纲设定，不要偏离核心情节
4. 保持与前文的连贯性
5. 开头要有吸引力，结尾要留有悬念或钩子
6. 避免使用 AI 味明显的表达，如"首先...其次...最后"、"值得注意的是"等
7. 注重人物心理描写和场景细节
8. 对话要自然，符合人物性格

请直接输出小说正文，不需要任何前缀说明。`, targetWordCount))

	return sb.String()
}

// ContinueChapter continues writing from existing content
func (w *Writer) ContinueChapter(ctx context.Context, existingContent, continuationPrompt string, targetWordCount int) (string, error) {
	prompt := fmt.Sprintf(`以下是一章小说的现有内容，请继续写作：

【现有内容】
%s

【续写要求】
%s

请直接输出续写内容，保持相同的写作风格和叙事节奏。`, existingContent, continuationPrompt)

	content, err := w.client.Generate(ctx, prompt,
		llm.WithTemperature(0.5),
		llm.WithMaxTokens(targetWordCount*2),
		llm.WithStream(true),
	)
	if err != nil {
		return "", fmt.Errorf("failed to continue chapter: %w", err)
	}
	content = strings.TrimSpace(content)
	if err := validateGeneratedContent(content); err != nil {
		return "", fmt.Errorf("invalid chapter continuation: %w", err)
	}

	return content, nil
}

// RewriteChapter rewrites a chapter with different emphasis
func (w *Writer) RewriteChapter(ctx context.Context, existingContent, rewriteInstructions string) (string, error) {
	prompt := fmt.Sprintf(`请根据以下要求重写这一章：

【现有内容】
%s

【重写要求】
%s

请直接输出重写后的内容，保持故事的核心情节不变。`, existingContent, rewriteInstructions)

	content, err := w.client.Generate(ctx, prompt,
		llm.WithTemperature(0.6),
		llm.WithMaxTokens(20000),
		llm.WithStream(true),
	)
	if err != nil {
		return "", fmt.Errorf("failed to rewrite chapter: %w", err)
	}
	content = strings.TrimSpace(content)
	if err := validateGeneratedContent(content); err != nil {
		return "", fmt.Errorf("invalid rewritten chapter: %w", err)
	}

	return content, nil
}

type ChapterSummary struct {
	Summary      string   `json:"summary"`
	KeyEvents    []string `json:"key_events"`
	Characters   []string `json:"characters"`
	EmotionalArc string   `json:"emotional_arc"`
}

// GenerateChapterSummary generates a summary for a chapter.
func (w *Writer) GenerateChapterSummary(ctx context.Context, chapterContent string) (string, error) {
	prompt := fmt.Sprintf(`请为以下章节生成一个简洁的摘要（200字以内）：

%s

请以 JSON 格式输出：
{
  "summary": "摘要内容...",
  "key_events": ["关键事件1", "关键事件2"],
  "characters": ["角色1", "角色2"],
  "emotional_arc": "情感弧线描述"
}`, chapterContent)

	result, err := w.client.GenerateJSON(ctx, prompt,
		llm.WithTemperature(0.5),
		llm.WithMaxTokens(2000),
	)
	if err != nil {
		return "", fmt.Errorf("failed to generate summary: %w", err)
	}

	summary, err := decodeChapterSummary(result)
	if err != nil {
		return "", fmt.Errorf("invalid summary response: %w", err)
	}
	return summary.Summary, nil
}

// GenerateChapterSummaryDetail returns a structured summary preview.
func (w *Writer) GenerateChapterSummaryDetail(ctx context.Context, chapterContent string) (*ChapterSummary, error) {
	prompt := fmt.Sprintf(`请为以下章节生成结构化摘要：

%s

请严格以 JSON 输出：
{
  "summary": "200字以内摘要",
  "key_events": ["关键事件"],
  "characters": ["角色"],
  "emotional_arc": "情感变化"
}`, chapterContent)
	result, err := w.client.GenerateJSON(ctx, prompt, llm.WithTemperature(0.5), llm.WithMaxTokens(2000))
	if err != nil {
		return nil, fmt.Errorf("failed to generate chapter summary: %w", err)
	}
	summary, err := decodeChapterSummary(result)
	if err != nil {
		return nil, fmt.Errorf("invalid chapter summary response: %w", err)
	}
	return summary, nil
}

func decodeChapterSummary(result map[string]interface{}) (*ChapterSummary, error) {
	summary, ok := result["summary"].(string)
	if !ok || strings.TrimSpace(summary) == "" {
		return nil, fmt.Errorf("summary must be a non-empty string")
	}
	emotionalArc, ok := result["emotional_arc"].(string)
	if !ok || strings.TrimSpace(emotionalArc) == "" {
		return nil, fmt.Errorf("emotional_arc must be a non-empty string")
	}
	keyEvents, err := requiredStringArray(result, "key_events")
	if err != nil {
		return nil, err
	}
	characters, err := requiredStringArray(result, "characters")
	if err != nil {
		return nil, err
	}
	return &ChapterSummary{Summary: strings.TrimSpace(summary), KeyEvents: keyEvents, Characters: characters, EmotionalArc: strings.TrimSpace(emotionalArc)}, nil
}

func requiredStringArray(result map[string]interface{}, key string) ([]string, error) {
	values, ok := result[key].([]interface{})
	if !ok || len(values) == 0 {
		return nil, fmt.Errorf("%s must be a non-empty string array", key)
	}
	items := make([]string, 0, len(values))
	for index, value := range values {
		item, ok := value.(string)
		if !ok || strings.TrimSpace(item) == "" {
			return nil, fmt.Errorf("%s[%d] must be a non-empty string", key, index)
		}
		items = append(items, strings.TrimSpace(item))
	}
	return items, nil
}

func validateGeneratedContent(content string) error {
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("generated content must not be empty")
	}
	return nil
}

// Helper function to clean chapter content
func cleanChapterContent(content string) string {
	// Remove any thinking tags or analysis
	content = strings.ReplaceAll(content, "<think>", "")
	content = strings.ReplaceAll(content, "</think>", "")

	// Remove leading/trailing whitespace
	content = strings.TrimSpace(content)

	// Remove common AI phrases
	aiPhrases := []string{
		"首先，", "其次，", "最后，",
		"值得注意的是，", "需要指出的是，",
		"总而言之，", "综上所述，",
	}
	for _, phrase := range aiPhrases {
		content = strings.ReplaceAll(content, phrase, "")
	}

	// Remove repeated newlines
	for strings.Contains(content, "\n\n\n") {
		content = strings.ReplaceAll(content, "\n\n\n", "\n\n")
	}

	return content
}
