package agents

import (
	"strings"
	"testing"

	"novel-agent/internal/models"
)

func TestStyleAnalyzerProducesUnicodeSafeProfile(t *testing.T) {
	content := "“快走！”林舟压低声音。\n雨落在青石板上，然而街角仍亮着一盏灯。"
	profile := NewStyleAnalyzer().Analyze(content)
	if profile.CharacterCount != len([]rune(content)) || profile.SentenceCount != 3 || profile.ParagraphCount != 2 {
		t.Fatalf("unexpected counts: %#v", profile)
	}
	if profile.DialogueRatio <= 0 || profile.VocabularyRichness <= 0 {
		t.Fatalf("expected dialogue and vocabulary metrics: %#v", profile)
	}
	if len(profile.CommonConnectives) != 1 || profile.CommonConnectives[0].Term != "然而" {
		t.Fatalf("unexpected connectives: %#v", profile.CommonConnectives)
	}
	if !profile.Preview || profile.AnalysisVersion != "style-heuristic-v1" {
		t.Fatalf("unexpected metadata: %#v", profile)
	}
}

func TestStyleAnalyzerComparesOnlyBoundedPreviousChapters(t *testing.T) {
	first := strings.Repeat("短句。", 500)
	second, future := "“走吧。”他转身离开。", "未来章节不应使用。"
	chapters := []models.Chapter{
		{ChapterNumber: 1, Content: &first},
		{ChapterNumber: 2, Content: &second},
		{ChapterNumber: 4, Content: &future},
	}
	result := NewStyleAnalyzer().CompareWithPrevious("“走吧。”她跟了上去。", chapters, 4, 1, 1000)
	if len(result.ReferenceChapterNumbers) != 1 || result.ReferenceChapterNumbers[0] != 2 {
		t.Fatalf("unexpected references: %#v", result.ReferenceChapterNumbers)
	}
	if result.ReferenceTruncated || result.Reference.CharacterCount == 0 || result.Similarity <= 0 {
		t.Fatalf("unexpected comparison: %#v", result)
	}
}

func TestStyleAnalyzerTruncatesReferenceRunes(t *testing.T) {
	content := strings.Repeat("章", 2000)
	chapters := []models.Chapter{{ChapterNumber: 1, Content: &content}}
	result := NewStyleAnalyzer().CompareWithPrevious("当前。", chapters, 2, 5, 1000)
	if !result.ReferenceTruncated || result.Reference.CharacterCount != 1000 {
		t.Fatalf("unexpected truncation: %#v", result)
	}
}

func TestStyleAnalyzerComparesCandidateWithExistingProfile(t *testing.T) {
	analyzer := NewStyleAnalyzer()
	reference := analyzer.Analyze("“快走。”他转身。\n雨落下来。")
	result := analyzer.CompareWithProfile("“快走。”她转身。\n风吹过来。", reference, []int{1, 2}, true)
	if result.Similarity <= 0 || len(result.ReferenceChapterNumbers) != 2 || !result.ReferenceTruncated {
		t.Fatalf("unexpected comparison: %#v", result)
	}
	result.ReferenceChapterNumbers[0] = 99
	if reference.CharacterCount == 0 {
		t.Fatal("reference profile unexpectedly empty")
	}
}

func TestStyleAnalyzerBuildsBoundedDeterministicGuide(t *testing.T) {
	analyzer := NewStyleAnalyzer()
	profile := analyzer.Analyze("然后出发。\n“走吧。”他回头。")
	guide := analyzer.BuildGuide(profile)
	for _, expected := range []string{"平均长度", "对话占比", "然后", "不要复制前文原句"} {
		if !strings.Contains(guide, expected) {
			t.Fatalf("guide %q missing %q", guide, expected)
		}
	}
	if analyzer.BuildGuide(StyleProfile{}) != "" {
		t.Fatal("empty profile must not produce a guide")
	}
}

func TestStyleAnalyzerEmptyAndDeterministic(t *testing.T) {
	analyzer := NewStyleAnalyzer()
	if profile := analyzer.Analyze(""); profile.CharacterCount != 0 || profile.DialogueRatio != 0 {
		t.Fatalf("unexpected empty profile: %#v", profile)
	}
	content := "然后出发。然后停下。此外，他回头。"
	first, second := analyzer.Analyze(content), analyzer.Analyze(content)
	if len(first.CommonConnectives) != len(second.CommonConnectives) || first.CommonConnectives[0] != second.CommonConnectives[0] {
		t.Fatalf("non-deterministic profiles: %#v %#v", first, second)
	}
}
