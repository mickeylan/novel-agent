package orchestration

import (
	"context"
	"testing"

	"novel-agent/internal/service/agents"
)

type fakeWriter struct{ content string }

func (f fakeWriter) WriteChapter(context.Context, string, string, string, string, int) (string, error) {
	return f.content, nil
}

type recordingWriter struct {
	content       string
	styleGuide    string
	memoryContext string
	called        bool
}

func (f *recordingWriter) WriteChapter(_ context.Context, _, styleGuide, memoryContext, _ string, _ int) (string, error) {
	f.called = true
	f.styleGuide = styleGuide
	f.memoryContext = memoryContext
	return f.content, nil
}

type fakeCritic struct {
	scores []float64
	index  int
}

func (f *fakeCritic) AuditChapter(context.Context, string, string, string) (*agents.AuditResult, error) {
	score := f.scores[f.index]
	if f.index < len(f.scores)-1 {
		f.index++
	}
	return &agents.AuditResult{OverallScore: score, Issues: []agents.Issue{{Severity: "warning", Description: "节奏偏慢"}}}, nil
}
func (f *fakeCritic) QuickAudit(context.Context, string) (float64, []string, error) {
	return 0, nil, nil
}

type fakeDataExtractor struct{}

func (fakeDataExtractor) ExtractPreview(_ context.Context, chapterNumber int, content, _ string) (*agents.DataExtractionPreview, error) {
	return &agents.DataExtractionPreview{StateChanges: []agents.StateChange{{EntityName: "林舟", Field: "状态", NewValue: content, Evidence: "正文", Confidence: 0.9}}, Preview: true}, nil
}

type fakeReviser struct{ candidate string }

func (f fakeReviser) Revise(context.Context, string, []agents.RevisionIssue, float64) (*agents.RevisionPreview, error) {
	return &agents.RevisionPreview{Candidate: f.candidate, Preview: true}, nil
}

func TestPreviewChapterRevisesOnceBelowThreshold(t *testing.T) {
	critic := &fakeCritic{scores: []float64{5.0, 7.5}}
	orchestrator := NewOrchestrator(fakeWriter{content: "初稿内容"}, critic, fakeReviser{candidate: "修订候选内容"}, agents.NewContinuityValidator(), agents.NewReaderPerspectiveAnalyzer(), nil, nil, nil, nil)
	result, err := orchestrator.PreviewChapter(context.Background(), PreviewRequest{Outline: "大纲", TargetWordCount: 1000, AutoRevise: true, RevisionThreshold: 6.0})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Revised || result.Candidate != "修订候选内容" || !result.Passed {
		t.Fatalf("unexpected workflow result: %#v", result)
	}
	if len(result.Steps) != 9 || !hasStep(result.Steps, "final_validate") || !hasStep(result.Steps, "final_reader_analysis") || !hasStep(result.Steps, "quality_gate") {
		t.Fatalf("expected revision and final quality steps: %#v", result.Steps)
	}
}

func TestPreviewChapterPassesMemoryContextToWriter(t *testing.T) {
	writer := &recordingWriter{content: "初稿"}
	critic := &fakeCritic{scores: []float64{8.0}}
	orchestrator := NewOrchestrator(writer, critic, fakeReviser{}, agents.NewContinuityValidator(), agents.NewReaderPerspectiveAnalyzer(), nil, nil, nil, nil)
	_, err := orchestrator.PreviewChapter(context.Background(), PreviewRequest{Outline: "大纲", StyleGuide: "保持短句", MemoryContext: "项目与角色上下文", TargetWordCount: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if writer.memoryContext != "项目与角色上下文" || writer.styleGuide != "保持短句" {
		t.Fatalf("writer received memory=%q style=%q", writer.memoryContext, writer.styleGuide)
	}
}

func TestPreviewChapterOptionallyExtractsDataChanges(t *testing.T) {
	critic := &fakeCritic{scores: []float64{8.0}}
	orchestrator := NewOrchestrator(fakeWriter{content: "候选正文"}, critic, fakeReviser{}, agents.NewContinuityValidator(), agents.NewReaderPerspectiveAnalyzer(), fakeDataExtractor{}, nil, nil, nil)
	result, err := orchestrator.PreviewChapter(context.Background(), PreviewRequest{Outline: "大纲", TargetWordCount: 1000, ChapterNumber: 3, ExtractDataChanges: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.DataChanges == nil || len(result.DataChanges.StateChanges) != 1 {
		t.Fatalf("expected data change preview: %#v", result.DataChanges)
	}
	if result.Steps[len(result.Steps)-1].Name != "data_extract" {
		t.Fatalf("expected data_extract step: %#v", result.Steps)
	}
}

func TestPreviewChapterChecksSceneCoverageWhenPlanProvided(t *testing.T) {
	critic := &fakeCritic{scores: []float64{8.0}}
	planner := agents.NewScenePlanner()
	plan := planner.Plan("", []string{"发现线索", "面对冲突"}, 1000, 8)
	orchestrator := NewOrchestrator(fakeWriter{content: "他发现线索，随后面对冲突。"}, critic, fakeReviser{}, agents.NewContinuityValidator(), agents.NewReaderPerspectiveAnalyzer(), nil, planner, nil, nil)
	result, err := orchestrator.PreviewChapter(context.Background(), PreviewRequest{Outline: "大纲", TargetWordCount: 1000, ScenePlan: &plan})
	if err != nil {
		t.Fatal(err)
	}
	if result.SceneCoverage == nil || result.SceneCoverage.CoveredScenes != 2 {
		t.Fatalf("expected scene coverage: %#v", result.SceneCoverage)
	}
	if !hasStep(result.Steps, "scene_coverage") {
		t.Fatalf("expected scene coverage step: %#v", result.Steps)
	}
}

func TestPreviewChapterOptionallyAnalyzesFinalCandidateAIGC(t *testing.T) {
	critic := &fakeCritic{scores: []float64{8.0}}
	detector := agents.NewAIGCDetector()
	orchestrator := NewOrchestrator(fakeWriter{content: "然而，他前进。然而，他继续。然而，他观察。然而，他等待。然而，他思考。然而，他离开。"}, critic, fakeReviser{}, agents.NewContinuityValidator(), agents.NewReaderPerspectiveAnalyzer(), nil, nil, detector, nil)
	result, err := orchestrator.PreviewChapter(context.Background(), PreviewRequest{Outline: "大纲", TargetWordCount: 1000, AnalyzeAIGC: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.AIGCAnalysis == nil || result.AIGCAnalysis.RiskLevel == "low" {
		t.Fatalf("expected AIGC analysis: %#v", result.AIGCAnalysis)
	}
	if !hasStep(result.Steps, "aigc_analysis") {
		t.Fatalf("expected aigc step: %#v", result.Steps)
	}
}

func TestPreviewChapterOptionallyComparesFinalCandidateStyle(t *testing.T) {
	critic := &fakeCritic{scores: []float64{8.0}}
	styleAnalyzer := agents.NewStyleAnalyzer()
	reference := styleAnalyzer.Analyze("“走吧。”他转身。\n雨落下来。")
	orchestrator := NewOrchestrator(fakeWriter{content: "“走吧。”她转身。\n风吹过来。"}, critic, fakeReviser{}, agents.NewContinuityValidator(), agents.NewReaderPerspectiveAnalyzer(), nil, nil, nil, styleAnalyzer)
	result, err := orchestrator.PreviewChapter(context.Background(), PreviewRequest{Outline: "大纲", TargetWordCount: 1000, StyleReference: &reference, StyleReferenceChapters: []int{1, 2}})
	if err != nil {
		t.Fatal(err)
	}
	if result.StyleComparison == nil || result.StyleComparison.Similarity <= 0 || len(result.StyleComparison.ReferenceChapterNumbers) != 2 {
		t.Fatalf("expected style comparison: %#v", result.StyleComparison)
	}
	if !hasStep(result.Steps, "style_comparison") {
		t.Fatalf("expected style comparison step: %#v", result.Steps)
	}
}

func TestEstimateOutputTokenBudgetUsesWorstCaseOptionalSteps(t *testing.T) {
	if got := EstimateOutputTokenBudget(5000, false, false); got != 14000 {
		t.Fatalf("base budget = %d, want 14000", got)
	}
	if got := EstimateOutputTokenBudget(5000, true, true); got != 32000 {
		t.Fatalf("full budget = %d, want 32000", got)
	}
}

func TestPreviewChapterRejectsInsufficientBudgetBeforeWriter(t *testing.T) {
	writer := &recordingWriter{content: "不应生成"}
	orchestrator := NewOrchestrator(writer, &fakeCritic{scores: []float64{8}}, fakeReviser{}, agents.NewContinuityValidator(), agents.NewReaderPerspectiveAnalyzer(), nil, nil, nil, nil)
	_, err := orchestrator.PreviewChapter(context.Background(), PreviewRequest{Outline: "大纲", TargetWordCount: 5000, MaxTotalTokens: 13000})
	if err == nil || writer.called {
		t.Fatalf("expected preflight budget rejection, got %v", err)
	}
}

func TestQualityGateIsAdvisoryAndAggregatesChecks(t *testing.T) {
	result := &ChapterWorkflowPreview{
		Passed: true, RevisionThreshold: 6, TargetWordCount: 1000, CandidateWordCount: 900, WordCountDeviation: 0.1, Audit: &agents.AuditResult{OverallScore: 8},
		ContinuityIssues: []agents.ContinuityIssue{{Severity: "warning"}},
		ReaderAnalysis:   agents.ReaderPerspectiveResult{AttractivenessScore: 8},
		SceneCoverage:    &agents.SceneCoverageResult{CoverageRate: 1, OrderValid: true},
		AIGCAnalysis:     &agents.AIGCDetectionResult{RiskLevel: "low", OverallScore: 8},
	}
	gate := buildQualityGate(result)
	if gate.RecommendedPass || !gate.Advisory || gate.PassedChecks != 5 || gate.TotalChecks != 6 {
		t.Fatalf("unexpected gate: %#v", gate)
	}
}

func TestQualityGateFlagsLargeWordCountDeviation(t *testing.T) {
	result := &ChapterWorkflowPreview{
		Passed: true, RevisionThreshold: 6, TargetWordCount: 1000,
		CandidateWordCount: 400, WordCountDeviation: 0.6,
		Audit:          &agents.AuditResult{OverallScore: 8},
		ReaderAnalysis: agents.ReaderPerspectiveResult{AttractivenessScore: 8},
	}
	gate := buildQualityGate(result)
	if gate.RecommendedPass || gate.Checks[1].Name != "word_count" || gate.Checks[1].Passed {
		t.Fatalf("unexpected word-count gate: %#v", gate)
	}
}

func hasStep(steps []StepResult, name string) bool {
	for _, step := range steps {
		if step.Name == name {
			return true
		}
	}
	return false
}

func TestPreviewChapterDoesNotRevisePassingDraft(t *testing.T) {
	critic := &fakeCritic{scores: []float64{8.0}}
	orchestrator := NewOrchestrator(fakeWriter{content: "合格初稿"}, critic, fakeReviser{candidate: "不应使用"}, agents.NewContinuityValidator(), agents.NewReaderPerspectiveAnalyzer(), nil, nil, nil, nil)
	result, err := orchestrator.PreviewChapter(context.Background(), PreviewRequest{Outline: "大纲", TargetWordCount: 1000, AutoRevise: true, RevisionThreshold: 6.0})
	if err != nil {
		t.Fatal(err)
	}
	if result.Revised || result.Candidate != "合格初稿" || !result.Passed {
		t.Fatalf("unexpected workflow result: %#v", result)
	}
}
