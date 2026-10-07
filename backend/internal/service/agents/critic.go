package agents

import (
	"context"
	"fmt"
	"strings"

	"novel-agent/internal/service/llm"
)

// Critic Agent - Quality auditing and review
type Critic struct {
	client *llm.Client
}

func NewCritic(client *llm.Client) *Critic {
	return &Critic{client: client}
}

// AuditResult represents the result of chapter auditing
type AuditResult struct {
	OverallScore float64     `json:"overall_score"` // 0-10
	Dimensions   []Dimension `json:"dimensions"`
	Issues       []Issue     `json:"issues"`
	Suggestions  []string    `json:"suggestions"`
	Summary      string      `json:"summary"`
}

type Dimension struct {
	Name     string  `json:"dimension"`
	Score    float64 `json:"score"` // 0-10
	Feedback string  `json:"feedback"`
}

type Issue struct {
	Type        string `json:"type"`     // consistency, pacing, style, etc.
	Severity    string `json:"severity"` // critical, warning, minor
	Description string `json:"description"`
	Location    string `json:"location"` // where the issue occurs
}

// AuditChapter performs a comprehensive audit of a chapter
func (c *Critic) AuditChapter(ctx context.Context, chapterContent, previousChapter, outline string) (*AuditResult, error) {
	prompt := fmt.Sprintf(`你是一位专业的小说评论家和编辑。请对以下章节进行全面的质量审核。

【审核要求】
请从以下几个维度对章节进行评分（每项 0-10 分）：
1. **情节连贯性**：故事逻辑是否通顺，与前文是否衔接自然
2. **人物塑造**：角色性格是否一致，对话是否符合人物特点
3. **节奏控制**：情节推进是否张弛有度，高潮是否有力
4. **语言风格**：文笔是否流畅，是否有AI味或重复表达
5. **细节描写**：场景、动作、心理描写是否到位
6. **情感表达**：是否能够引发读者共鸣

【前章结尾】（用于检查连贯性）
%s

【本章大纲】
%s

【待审核章节】
%s

请以 JSON 格式输出审核结果：
{
  "overall_score": 8.5,
  "dimensions": [
    {"name": "情节连贯性", "score": 9.0, "feedback": "..."},
    {"name": "人物塑造", "score": 8.0, "feedback": "..."},
    {"name": "节奏控制", "score": 8.5, "feedback": "..."},
    {"name": "语言风格", "score": 8.0, "feedback": "..."},
    {"name": "细节描写", "score": 7.5, "feedback": "..."},
    {"name": "情感表达", "score": 8.0, "feedback": "..."}
  ],
  "issues": [
    {"type": "风格", "severity": "minor", "description": "出现重复表达", "location": "第3段"}
  ],
  "suggestions": ["建议加强人物心理描写", "注意避免重复用词"],
  "summary": "总体评价..."
}`, previousChapter, outline, chapterContent)

	result, err := c.client.GenerateJSON(ctx, prompt,
		llm.WithTemperature(0.5),
		llm.WithMaxTokens(4000),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to audit chapter: %w", err)
	}

	audit, err := parseAuditResult(result)
	if err != nil {
		return nil, fmt.Errorf("invalid audit response: %w", err)
	}
	return audit, nil
}

// QuickAudit performs a quick quality check
func (c *Critic) QuickAudit(ctx context.Context, chapterContent string) (float64, []string, error) {
	prompt := fmt.Sprintf(`请快速评估以下章节的质量（只需给出分数和建议）：

【待审核章节】
%s

请以 JSON 格式输出：
{
  "score": 8.5,
  "issues": ["问题1", "问题2"],
  "suggestions": ["建议1", "建议2"]
}`, chapterContent)

	result, err := c.client.GenerateJSON(ctx, prompt,
		llm.WithTemperature(0.5),
		llm.WithMaxTokens(2000),
	)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to quick audit: %w", err)
	}

	score, ok := result["score"].(float64)
	if !ok || score < 0 || score > 10 {
		return 0, nil, fmt.Errorf("invalid quick audit response: score must be between 0 and 10")
	}
	suggestions, err := requiredStringArray(result, "suggestions")
	if err != nil {
		return 0, nil, fmt.Errorf("invalid quick audit response: %w", err)
	}
	return score, suggestions, nil
}

// SuggestImprovements generates improvement suggestions for a chapter
func (c *Critic) SuggestImprovements(ctx context.Context, chapterContent, auditReport string) ([]string, error) {
	prompt := fmt.Sprintf(`基于以下审核报告，请为章节提供具体的改进建议：

【审核报告】
%s

【章节内容】
%s

请列出 3-5 条具体的改进建议，每条建议要明确指出问题和解决方案。`, auditReport, chapterContent)

	suggestions, err := c.client.Generate(ctx, prompt,
		llm.WithTemperature(0.6),
		llm.WithMaxTokens(2000),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to generate improvements: %w", err)
	}

	// Parse suggestions into array
	lines := strings.Split(suggestions, "\n")
	var result []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && (strings.HasPrefix(line, "-") || strings.HasPrefix(line, "•") || strings.HasPrefix(line, "·")) {
			result = append(result, strings.TrimPrefix(strings.TrimPrefix(line, "-"), "•"))
		}
	}

	return result, nil
}

// CheckContinuity checks for continuity errors with previous chapters
func (c *Critic) CheckContinuity(ctx context.Context, currentChapter, previousChapters string) ([]string, error) {
	prompt := fmt.Sprintf(`请检查当前章节与前文之间是否存在以下类型的连贯性问题：

1. 人物状态不一致（如：之前受伤现在完好）
2. 物品/道具消失或出现矛盾
3. 时间线错误
4. 地理/场景位置混乱
5. 情节逻辑矛盾

【前文内容摘要】
%s

【当前章节】
%s

请列出发现的连贯性问题，每个问题请说明：
- 问题类型
- 具体描述
- 建议的修复方式

如果没有发现问题，请回复"未发现连贯性问题"。`, previousChapters, currentChapter)

	result, err := c.client.Generate(ctx, prompt,
		llm.WithTemperature(0.5),
		llm.WithMaxTokens(3000),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to check continuity: %w", err)
	}

	if strings.Contains(result, "未发现") || strings.Contains(result, "没有问题") {
		return []string{}, nil
	}

	// Parse into array
	var issues []string
	lines := strings.Split(result, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && len(line) > 10 {
			issues = append(issues, line)
		}
	}

	return issues, nil
}

// Helper function to parse audit result
func parseAuditResult(m map[string]interface{}) (*AuditResult, error) {
	score, ok := m["overall_score"].(float64)
	if !ok || score < 0 || score > 10 {
		return nil, fmt.Errorf("overall_score must be between 0 and 10")
	}
	summary, ok := m["summary"].(string)
	if !ok || strings.TrimSpace(summary) == "" {
		return nil, fmt.Errorf("summary must be a non-empty string")
	}
	dims, ok := m["dimensions"].([]interface{})
	if !ok || len(dims) == 0 {
		return nil, fmt.Errorf("dimensions must be a non-empty array")
	}
	result := &AuditResult{OverallScore: score, Summary: strings.TrimSpace(summary), Dimensions: make([]Dimension, 0, len(dims))}
	for index, raw := range dims {
		dimensionMap, ok := raw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("dimensions[%d] must be an object", index)
		}
		name, feedback := strings.TrimSpace(getStringValue(dimensionMap, "name")), strings.TrimSpace(getStringValue(dimensionMap, "feedback"))
		dimensionScore, ok := dimensionMap["score"].(float64)
		if name == "" || feedback == "" || !ok || dimensionScore < 0 || dimensionScore > 10 {
			return nil, fmt.Errorf("dimensions[%d] requires name, feedback, and score between 0 and 10", index)
		}
		result.Dimensions = append(result.Dimensions, Dimension{Name: name, Score: dimensionScore, Feedback: feedback})
	}
	issues, ok := m["issues"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("issues must be an array")
	}
	result.Issues = make([]Issue, 0, len(issues))
	for index, raw := range issues {
		issueMap, ok := raw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("issues[%d] must be an object", index)
		}
		issue := Issue{Type: strings.TrimSpace(getStringValue(issueMap, "type")), Severity: strings.ToLower(strings.TrimSpace(getStringValue(issueMap, "severity"))), Description: strings.TrimSpace(getStringValue(issueMap, "description")), Location: strings.TrimSpace(getStringValue(issueMap, "location"))}
		if issue.Type == "" || issue.Description == "" || issue.Location == "" || (issue.Severity != "critical" && issue.Severity != "warning" && issue.Severity != "minor") {
			return nil, fmt.Errorf("issues[%d] has invalid required fields", index)
		}
		result.Issues = append(result.Issues, issue)
	}
	suggestions, err := requiredStringArray(m, "suggestions")
	if err != nil {
		return nil, err
	}
	result.Suggestions = suggestions
	return result, nil
}

func getStringValue(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
