package agents

import (
	"strings"
	"testing"
)

func TestAIGCDetectorFindsTransitionOveruse(t *testing.T) {
	content := strings.Repeat("然而，他继续向前。", 7)
	result := NewAIGCDetector().Detect(content)
	if result.RiskLevel == "low" {
		t.Fatalf("expected elevated risk: %#v", result)
	}
	found := false
	for _, pattern := range result.Patterns {
		if pattern.Type == "transition_overuse" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected transition pattern: %#v", result.Patterns)
	}
	if !result.Preview || result.AnalysisVersion != "heuristic-v1" {
		t.Fatalf("unexpected metadata: %#v", result)
	}
}

func TestAIGCDetectorHandlesUnicodeAndEmptyContent(t *testing.T) {
	detector := NewAIGCDetector()
	if result := detector.Detect(""); result.CharacterCount != 0 || result.SentenceCount != 0 {
		t.Fatalf("unexpected empty result: %#v", result)
	}
	result := detector.Detect("雨落在青石板上。林舟停住脚步！")
	if result.CharacterCount != len([]rune("雨落在青石板上。林舟停住脚步！")) || result.SentenceCount != 2 {
		t.Fatalf("unexpected unicode counts: %#v", result)
	}
}
