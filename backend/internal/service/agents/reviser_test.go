package agents

import (
	"strings"
	"testing"
)

func TestBuildRevisionPromptLimitsIssuesAndContent(t *testing.T) {
	issues := make([]RevisionIssue, 7)
	for i := range issues {
		issues[i] = RevisionIssue{Problem: "问题", Suggestion: "建议", Severity: "warning"}
	}
	prompt := BuildRevisionPrompt(strings.Repeat("𠮷", 8000), issues, 6.5)
	if strings.Count(prompt, "[WARNING]") != 5 {
		t.Fatalf("expected 5 issues in prompt")
	}
	if !strings.Contains(prompt, "6.5/10.0") {
		t.Fatal("score missing from prompt")
	}
	if strings.Count(prompt, "𠮷") != 7000 {
		t.Fatalf("expected rune-safe 7000-character content limit")
	}
}
