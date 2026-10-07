package agents

import (
	"fmt"
	"strings"

	"novel-agent/internal/models"
)

type WritingContext struct {
	Project        string   `json:"project"`
	Characters     []string `json:"characters"`
	RecentChapters []string `json:"recent_chapters"`
	PromptContext  string   `json:"prompt_context"`
	CharacterCount int      `json:"character_count"`
	ChapterCount   int      `json:"chapter_count"`
	Truncated      bool     `json:"truncated"`
}

type ContextBuilder struct{}

func NewContextBuilder() *ContextBuilder { return &ContextBuilder{} }

func (b *ContextBuilder) Build(project *models.StoryProject, characters []models.Character, chapters []models.Chapter, beforeChapter, maxRunes int) WritingContext {
	if maxRunes <= 0 || maxRunes > 12000 {
		maxRunes = 8000
	}
	projectText := fmt.Sprintf("标题：%s\n题材：%s\n故事前提：%s\n世界设定：%s", project.Title, strings.Join(project.Genre, "、"), value(project.Premise), value(project.WorldSetting))
	characterTexts := make([]string, 0, len(characters))
	for _, character := range characters {
		characterTexts = append(characterTexts, fmt.Sprintf("%s｜特征：%s｜当前状态：%s｜人物弧：%s", character.Name, strings.Join(character.Traits, "、"), value(character.CurrentState), value(character.ArcSummary)))
	}
	recent := make([]string, 0, 3)
	for index := len(chapters) - 1; index >= 0 && len(recent) < 3; index-- {
		chapter := chapters[index]
		if chapter.ChapterNumber >= beforeChapter {
			continue
		}
		summary := value(chapter.Summary)
		if summary == "" {
			summary = value(chapter.Outline)
		}
		if summary == "" && chapter.Content != nil {
			summary = firstRunes(*chapter.Content, 400)
		}
		recent = append([]string{fmt.Sprintf("第%d章 %s：%s", chapter.ChapterNumber, value(chapter.Title), summary)}, recent...)
	}

	var builder strings.Builder
	builder.WriteString("【项目设定】\n" + projectText + "\n\n")
	if len(characterTexts) > 0 {
		builder.WriteString("【角色状态】\n" + strings.Join(characterTexts, "\n") + "\n\n")
	}
	if len(recent) > 0 {
		builder.WriteString("【最近章节】\n" + strings.Join(recent, "\n") + "\n")
	}
	promptRunes := []rune(builder.String())
	truncated := len(promptRunes) > maxRunes
	if truncated {
		promptRunes = promptRunes[:maxRunes]
	}
	return WritingContext{Project: projectText, Characters: characterTexts, RecentChapters: recent, PromptContext: string(promptRunes), CharacterCount: len(characterTexts), ChapterCount: len(recent), Truncated: truncated}
}

func value(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
