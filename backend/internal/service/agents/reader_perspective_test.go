package agents

import (
	"strings"
	"testing"
)

func TestReaderPerspectiveShortContent(t *testing.T) {
	result := NewReaderPerspectiveAnalyzer().Analyze("短文本")
	if result.HookStrength != 0 || result.ClimaxDensity != 0 {
		t.Fatalf("short content scores must be zero: %#v", result)
	}
	if len(result.EmotionCurve) != 10 {
		t.Fatalf("expected ten emotion points, got %d", len(result.EmotionCurve))
	}
}

func TestReaderPerspectiveDetectsForbiddenEnding(t *testing.T) {
	line := strings.Repeat("危机正在逼近，众人紧张地看向未知的门后。", 3)
	content := strings.Join([]string{line, line, line, line, line, "一切才刚刚开始"}, "\n")
	result := NewReaderPerspectiveAnalyzer().Analyze(content)
	found := false
	for _, issue := range result.Issues {
		if issue.Type == "forbidden_ending" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected forbidden ending issue")
	}
}

func TestReaderPerspectiveCountsUnicodeByRunes(t *testing.T) {
	content := strings.Repeat("突破", 50) + strings.Repeat("𠮷", 100)
	result := NewReaderPerspectiveAnalyzer().Analyze(content)
	if result.ClimaxDensity <= 0 {
		t.Fatal("expected positive climax density")
	}
}
