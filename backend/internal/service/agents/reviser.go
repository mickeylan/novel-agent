package agents

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"novel-agent/internal/service/llm"
)

type RevisionIssue struct {
	Problem    string `json:"problem"`
	Suggestion string `json:"suggestion"`
	Severity   string `json:"severity"`
}

type RevisionPreview struct {
	Original       string          `json:"original"`
	Candidate      string          `json:"candidate"`
	OriginalScore  float64         `json:"original_score"`
	CandidateScore *float64        `json:"candidate_score,omitempty"`
	Issues         []RevisionIssue `json:"issues"`
	Preview        bool            `json:"preview"`
	Fallback       bool            `json:"fallback"`
}

type Reviser struct {
	client *llm.Client
}

func NewReviser(client *llm.Client) *Reviser { return &Reviser{client: client} }

func (r *Reviser) Revise(ctx context.Context, content string, issues []RevisionIssue, overallScore float64) (*RevisionPreview, error) {
	prompt := BuildRevisionPrompt(content, issues, overallScore)
	candidate, err := r.client.Generate(ctx, prompt, llm.WithTemperature(0.7), llm.WithMaxTokens(8000))
	if err != nil {
		return nil, fmt.Errorf("failed to revise chapter: %w", err)
	}
	candidate = strings.TrimSpace(candidate)
	fallback := candidate == "" || utf8.RuneCountInString(candidate) < 100
	if fallback {
		candidate = content
	}
	return &RevisionPreview{Original: content, Candidate: candidate, OriginalScore: overallScore, Issues: issues, Preview: true, Fallback: fallback}, nil
}

func BuildRevisionPrompt(content string, issues []RevisionIssue, overallScore float64) string {
	var issueList strings.Builder
	limit := len(issues)
	if limit > 5 {
		limit = 5
	}
	for i := 0; i < limit; i++ {
		issue := issues[i]
		severity := issue.Severity
		if severity == "" {
			severity = "medium"
		}
		problem := issue.Problem
		if problem == "" {
			problem = "未指定问题"
		}
		suggestion := issue.Suggestion
		if suggestion == "" {
			suggestion = "请改进"
		}
		fmt.Fprintf(&issueList, "%d. [%s] %s\n   建议：%s\n\n", i+1, strings.ToUpper(severity), problem, suggestion)
	}

	contentRunes := []rune(content)
	if len(contentRunes) > 7000 {
		contentRunes = contentRunes[:7000]
	}
	return fmt.Sprintf(`你是一位专业的小说编辑，需要对以下章节进行修订。

## 章节信息
- 当前评分：%.1f/10.0

## 审计发现的问题
%s
## 原始内容
%s

## 修订要求
1. 针对上述问题进行针对性修改
2. 保持原有文风和叙事节奏
3. 不要改变核心剧情和人物设定
4. 修改幅度控制在 20-40%% 之间
5. 确保修订后内容流畅自然

直接输出修订后的完整章节内容，不要包含任何说明文字。`, overallScore, issueList.String(), string(contentRunes))
}
