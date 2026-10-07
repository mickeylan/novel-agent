package agents

import (
	"strings"
	"testing"

	"novel-agent/internal/models"
)

func TestContextBuilderUsesOnlyPriorChaptersAndBoundsOutput(t *testing.T) {
	premise, state := "故事前提", "受伤"
	project := &models.StoryProject{Title: "测试小说", Genre: []string{"玄幻"}, Premise: &premise}
	characters := []models.Character{{Name: "林舟", Traits: []string{"谨慎"}, CurrentState: &state}}
	content := strings.Repeat("章", 1000)
	chapters := []models.Chapter{{ChapterNumber: 1, Content: &content}, {ChapterNumber: 3, Content: &content}}
	result := NewContextBuilder().Build(project, characters, chapters, 3, 200)
	if result.ChapterCount != 1 || !result.Truncated {
		t.Fatalf("unexpected context: %#v", result)
	}
	if len([]rune(result.PromptContext)) != 200 {
		t.Fatalf("expected 200 runes")
	}
}
