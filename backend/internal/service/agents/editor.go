package agents

import (
	"context"
	"fmt"
	"strings"

	"novel-agent/internal/service/llm"
)

func validateEditorResult(result string) (string, error) {
	result = strings.TrimSpace(result)
	if result == "" {
		return "", fmt.Errorf("editor result must not be empty")
	}
	return result, nil
}

// Editor Agent - Style polishing and consistency checking
type Editor struct {
	client *llm.Client
}

func NewEditor(client *llm.Client) *Editor {
	return &Editor{client: client}
}

// PolishChapter polishes a chapter for style and flow
func (e *Editor) PolishChapter(ctx context.Context, chapterContent, styleGuide string) (string, error) {
	prompt := fmt.Sprintf(`你是一位专业的小说编辑和润色专家。请对以下章节进行润色，提升文笔质量。

【润色要求】
1. 保持原文的核心内容和情节
2. 改善语言表达，消除生硬翻译感
3. 优化句式结构，增加阅读节奏感
4. 减少重复用词，增加表达多样性
5. 保持中文特有的文学韵味

【风格指南】（如有）
%s

【待润色章节】
%s

请直接输出润色后的内容，不需要说明修改了什么。`, styleGuide, chapterContent)

	result, err := e.client.Generate(ctx, prompt,
		llm.WithTemperature(0.6),
		llm.WithMaxTokens(20000),
		llm.WithStream(true),
	)
	if err != nil {
		return "", fmt.Errorf("failed to polish chapter: %w", err)
	}

	return validateEditorResult(result)
}

// HumanizeContent removes AI flavor from content
func (e *Editor) HumanizeContent(ctx context.Context, content string) (string, error) {
	prompt := fmt.Sprintf(`请将以下内容改写得更加自然，消除AI写作的痕迹。

【需要改写的内容】
%s

【改写要求】
1. 改变句式结构，避免过于规整
2. 使用更加口语化和自然的表达
3. 保留核心信息，但改变表达方式
4. 添加适当的省略和跳跃
5. 保持原文的长度

请直接输出改写后的内容。`, content)

	result, err := e.client.Generate(ctx, prompt,
		llm.WithTemperature(0.7),
		llm.WithMaxTokens(20000),
		llm.WithStream(true),
	)
	if err != nil {
		return "", fmt.Errorf("failed to humanize content: %w", err)
	}

	return validateEditorResult(result)
}

// CheckForbiddenPatterns checks for forbidden writing patterns
func (e *Editor) CheckForbiddenPatterns(ctx context.Context, content string) ([]string, error) {
	forbiddenPatterns := []string{
		"首先，", "其次，", "最后，",
		"值得注意的是，", "需要指出的是，",
		"总而言之，", "综上所述，",
		"换句话说", "也就是说",
		"从这个角度来看", "从某种意义上说",
		"众所周知", "不得不承认",
	}

	var found []string
	for _, pattern := range forbiddenPatterns {
		if strings.Contains(content, pattern) {
			found = append(found, pattern)
		}
	}

	return found, nil
}

// FixConsistencyIssues fixes consistency issues in content
func (e *Editor) FixConsistencyIssues(ctx context.Context, content, issues string) (string, error) {
	prompt := fmt.Sprintf(`请修复以下章节中的连贯性问题：

【发现的问题】
%s

【原始内容】
%s

请直接输出修复后的内容，保持故事的流畅性。`, issues, content)

	result, err := e.client.Generate(ctx, prompt,
		llm.WithTemperature(0.5),
		llm.WithMaxTokens(20000),
		llm.WithStream(true),
	)
	if err != nil {
		return "", fmt.Errorf("failed to fix consistency: %w", err)
	}

	return validateEditorResult(result)
}

// EnhanceDialogue enhances dialogue to be more natural
func (e *Editor) EnhanceDialogue(ctx context.Context, content string) (string, error) {
	prompt := fmt.Sprintf(`请优化以下章节中的对话部分，使其更加自然生动：

【原文】
%s

【优化要求】
1. 对话要符合人物性格和身份
2. 加入适当的省略和跳跃
3. 增加对话的张力和信息量
4. 减少废话，增加含金量
5. 加入适当的面部表情和肢体语言描写

请直接输出优化后的内容。`, content)

	result, err := e.client.Generate(ctx, prompt,
		llm.WithTemperature(0.6),
		llm.WithMaxTokens(20000),
		llm.WithStream(true),
	)
	if err != nil {
		return "", fmt.Errorf("failed to enhance dialogue: %w", err)
	}

	return validateEditorResult(result)
}

// EnhanceDescription enhances scene and action descriptions
func (e *Editor) EnhanceDescription(ctx context.Context, content string) (string, error) {
	prompt := fmt.Sprintf(`请增强以下章节的场景和动作描写：

【原文】
%s

【增强要求】
1. 场景描写要具体、有画面感
2. 动作描写要精准、有力度
3. 适当加入感官细节（视觉、听觉、嗅觉等）
4. 避免冗长，保持节奏
5. 与情节紧密结合

请直接输出增强后的内容。`, content)

	result, err := e.client.Generate(ctx, prompt,
		llm.WithTemperature(0.6),
		llm.WithMaxTokens(20000),
		llm.WithStream(true),
	)
	if err != nil {
		return "", fmt.Errorf("failed to enhance description: %w", err)
	}

	return validateEditorResult(result)
}
