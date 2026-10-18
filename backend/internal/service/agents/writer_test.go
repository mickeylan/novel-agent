package agents

import (
	"strings"
	"testing"
)

func TestValidateGeneratedContentRejectsWhitespace(t *testing.T) {
	for _, content := range []string{"", "   ", "\n\t"} {
		if err := validateGeneratedContent(content); err == nil {
			t.Fatalf("whitespace generated content was accepted: %q", content)
		}
	}
	if err := validateGeneratedContent("有效正文"); err != nil {
		t.Fatalf("valid generated content rejected: %v", err)
	}
}

func TestCleanChapterContentCannotSilentlyProduceEmptySuccess(t *testing.T) {
	cleaned := cleanChapterContent("  \n\t  ")
	if err := validateGeneratedContent(cleaned); err == nil {
		t.Fatal("empty post-processed content was accepted")
	}
}

func TestDecodeChapterSummaryValidatesRequiredSchema(t *testing.T) {
	valid := map[string]interface{}{
		"summary": "  本章摘要  ", "emotional_arc": " 紧张到释然 ",
		"key_events": []interface{}{" 发现线索 "}, "characters": []interface{}{" 林默 "},
	}
	result, err := decodeChapterSummary(valid)
	if err != nil {
		t.Fatal(err)
	}
	if result.Summary != "本章摘要" || result.KeyEvents[0] != "发现线索" || result.Characters[0] != "林默" {
		t.Fatalf("unexpected normalized summary: %#v", result)
	}

	invalid := []map[string]interface{}{
		{"emotional_arc": "变化", "key_events": []interface{}{"事件"}, "characters": []interface{}{"角色"}},
		{"summary": 123, "emotional_arc": "变化", "key_events": []interface{}{"事件"}, "characters": []interface{}{"角色"}},
		{"summary": "摘要", "emotional_arc": "变化", "key_events": []interface{}{}, "characters": []interface{}{"角色"}},
		{"summary": "摘要", "emotional_arc": "变化", "key_events": []interface{}{"事件"}, "characters": []interface{}{1}},
	}
	for index, value := range invalid {
		if _, err := decodeChapterSummary(value); err == nil {
			t.Fatalf("invalid schema %d was accepted: %#v", index, value)
		}
	}
}

func TestBuildWritingPromptIncludesContext(t *testing.T) {
	writer := &Writer{}
	prompt := writer.BuildWritingPrompt("章节大纲", "风格指南", "记忆上下文", "前章结尾", 3200)
	for _, expected := range []string{"章节大纲", "风格指南", "记忆上下文", "前章结尾", "3200"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("prompt does not contain %q", expected)
		}
	}
}
