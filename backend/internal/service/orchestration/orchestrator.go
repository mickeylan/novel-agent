package orchestration

import (
	"context"
	"fmt"
	"math"
	"time"
	"unicode/utf8"

	"novel-agent/internal/service/agents"
)

type Writer interface {
	WriteChapter(ctx context.Context, outline, styleGuide, memoryContext, previousChapter string, targetWordCount int) (string, error)
}

type Critic interface {
	AuditChapter(ctx context.Context, chapterContent, previousChapter, outline string) (*agents.AuditResult, error)
	QuickAudit(ctx context.Context, chapterContent string) (float64, []string, error)
}

type Reviser interface {
	Revise(ctx context.Context, content string, issues []agents.RevisionIssue, overallScore float64) (*agents.RevisionPreview, error)
}

type DataExtractor interface {
	ExtractPreview(ctx context.Context, chapterNumber int, content, knownContext string) (*agents.DataExtractionPreview, error)
}

type PreviewRequest struct {
	Outline                 string
	StyleGuide              string
	MemoryContext           string
	PreviousChapter         string
	NewCharacters           []string
	TargetWordCount         int
	AutoRevise              bool
	RevisionThreshold       float64
	ChapterNumber           int
	ExtractDataChanges      bool
	ScenePlan               *agents.ScenePlanPreview
	AnalyzeAIGC             bool
	StyleReference          *agents.StyleProfile
	StyleReferenceChapters  []int
	StyleReferenceTruncated bool
}

type StepResult struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	DurationMS int64  `json:"duration_ms"`
	Error      string `json:"error,omitempty"`
}

type QualityCheck struct {
	Name    string  `json:"name"`
	Passed  bool    `json:"passed"`
	Score   float64 `json:"score,omitempty"`
	Message string  `json:"message"`
}

type QualityGate struct {
	RecommendedPass bool           `json:"recommended_pass"`
	Checks          []QualityCheck `json:"checks"`
	PassedChecks    int            `json:"passed_checks"`
	TotalChecks     int            `json:"total_checks"`
	Advisory        bool           `json:"advisory"`
}

type ChapterWorkflowPreview struct {
	Draft              string                         `json:"draft"`
	Candidate          string                         `json:"candidate"`
	Audit              *agents.AuditResult            `json:"audit"`
	CandidateAudit     *agents.AuditResult            `json:"candidate_audit,omitempty"`
	ContinuityIssues   []agents.ContinuityIssue       `json:"continuity_issues"`
	ReaderAnalysis     agents.ReaderPerspectiveResult `json:"reader_analysis"`
	Revision           *agents.RevisionPreview        `json:"revision,omitempty"`
	Revised            bool                           `json:"revised"`
	Passed             bool                           `json:"passed"`
	RevisionThreshold  float64                        `json:"revision_threshold"`
	TargetWordCount    int                            `json:"target_word_count"`
	CandidateWordCount int                            `json:"candidate_word_count"`
	WordCountDeviation float64                        `json:"word_count_deviation"`
	Steps              []StepResult                   `json:"steps"`
	Preview            bool                           `json:"preview"`
	ExecutionTimeMS    int64                          `json:"execution_time_ms"`
	DataChanges        *agents.DataExtractionPreview  `json:"data_changes,omitempty"`
	SceneCoverage      *agents.SceneCoverageResult    `json:"scene_coverage,omitempty"`
	AIGCAnalysis       *agents.AIGCDetectionResult    `json:"aigc_analysis,omitempty"`
	StyleComparison    *agents.StyleComparison        `json:"style_comparison,omitempty"`
	QualityGate        QualityGate                    `json:"quality_gate"`
	Errors             []string                       `json:"errors"`
}

type Orchestrator struct {
	writer        Writer
	critic        Critic
	reviser       Reviser
	validator     *agents.ContinuityValidator
	reader        *agents.ReaderPerspectiveAnalyzer
	dataAgent     DataExtractor
	scenePlanner  *agents.ScenePlanner
	aigcDetector  *agents.AIGCDetector
	styleAnalyzer *agents.StyleAnalyzer
}

func NewOrchestrator(writer Writer, critic Critic, reviser Reviser, validator *agents.ContinuityValidator, reader *agents.ReaderPerspectiveAnalyzer, dataAgent DataExtractor, scenePlanner *agents.ScenePlanner, aigcDetector *agents.AIGCDetector, styleAnalyzer *agents.StyleAnalyzer) *Orchestrator {
	return &Orchestrator{writer: writer, critic: critic, reviser: reviser, validator: validator, reader: reader, dataAgent: dataAgent, scenePlanner: scenePlanner, aigcDetector: aigcDetector, styleAnalyzer: styleAnalyzer}
}

func (o *Orchestrator) PreviewChapter(ctx context.Context, request PreviewRequest) (*ChapterWorkflowPreview, error) {
	started := time.Now()
	if request.TargetWordCount == 0 {
		request.TargetWordCount = 5000
	}
	if request.TargetWordCount < 100 || request.TargetWordCount > 20000 {
		return nil, fmt.Errorf("target word count must be between 100 and 20000")
	}
	if request.RevisionThreshold == 0 {
		request.RevisionThreshold = 6.0
	}
	if request.RevisionThreshold < 0 || request.RevisionThreshold > 10 {
		return nil, fmt.Errorf("revision threshold must be between 0 and 10")
	}

	result := &ChapterWorkflowPreview{RevisionThreshold: request.RevisionThreshold, TargetWordCount: request.TargetWordCount, Steps: make([]StepResult, 0, 11), Errors: []string{}, Preview: true}

	draft, err := runStep(&result.Steps, "write", func() (string, error) {
		return o.writer.WriteChapter(ctx, request.Outline, request.StyleGuide, request.MemoryContext, request.PreviousChapter, request.TargetWordCount)
	})
	if err != nil {
		return nil, err
	}
	result.Draft, result.Candidate = draft, draft

	stepStarted := time.Now()
	result.ContinuityIssues = o.validator.ValidateChapter(draft, request.PreviousChapter, request.NewCharacters)
	result.Steps = append(result.Steps, StepResult{Name: "validate", Status: "completed", DurationMS: time.Since(stepStarted).Milliseconds()})

	stepStarted = time.Now()
	result.ReaderAnalysis = o.reader.Analyze(draft)
	result.Steps = append(result.Steps, StepResult{Name: "reader_analysis", Status: "completed", DurationMS: time.Since(stepStarted).Milliseconds()})

	stepStarted = time.Now()
	result.Audit, err = o.critic.AuditChapter(ctx, draft, request.PreviousChapter, request.Outline)
	result.Steps = append(result.Steps, stepFromError("audit", stepStarted, err))
	if err != nil {
		return nil, err
	}
	result.Passed = result.Audit.OverallScore >= request.RevisionThreshold

	if request.AutoRevise && !result.Passed {
		issues := revisionIssues(result.Audit)
		stepStarted = time.Now()
		result.Revision, err = o.reviser.Revise(ctx, draft, issues, result.Audit.OverallScore)
		result.Steps = append(result.Steps, stepFromError("revise", stepStarted, err))
		if err != nil {
			return nil, err
		}
		if !result.Revision.Fallback && result.Revision.Candidate != draft {
			result.Candidate = result.Revision.Candidate
			result.Revised = true
			stepStarted = time.Now()
			result.CandidateAudit, err = o.critic.AuditChapter(ctx, result.Candidate, request.PreviousChapter, request.Outline)
			result.Steps = append(result.Steps, stepFromError("reaudit", stepStarted, err))
			if err != nil {
				return nil, err
			}
			result.Passed = result.CandidateAudit.OverallScore >= request.RevisionThreshold
		}
	}

	if request.ScenePlan != nil && o.scenePlanner != nil {
		stepStarted = time.Now()
		coverage := o.scenePlanner.ValidateCoverage(result.Candidate, *request.ScenePlan)
		result.SceneCoverage = &coverage
		result.Steps = append(result.Steps, StepResult{Name: "scene_coverage", Status: "completed", DurationMS: time.Since(stepStarted).Milliseconds()})
	}

	if request.StyleReference != nil && o.styleAnalyzer != nil {
		stepStarted = time.Now()
		comparison := o.styleAnalyzer.CompareWithProfile(result.Candidate, *request.StyleReference, request.StyleReferenceChapters, request.StyleReferenceTruncated)
		result.StyleComparison = &comparison
		result.Steps = append(result.Steps, StepResult{Name: "style_comparison", Status: "completed", DurationMS: time.Since(stepStarted).Milliseconds()})
	}

	if request.AnalyzeAIGC && o.aigcDetector != nil {
		stepStarted = time.Now()
		analysis := o.aigcDetector.Detect(result.Candidate)
		result.AIGCAnalysis = &analysis
		result.Steps = append(result.Steps, StepResult{Name: "aigc_analysis", Status: "completed", DurationMS: time.Since(stepStarted).Milliseconds()})
	}

	if result.Revised {
		stepStarted = time.Now()
		result.ContinuityIssues = o.validator.ValidateChapter(result.Candidate, request.PreviousChapter, request.NewCharacters)
		result.Steps = append(result.Steps, StepResult{Name: "final_validate", Status: "completed", DurationMS: time.Since(stepStarted).Milliseconds()})
		stepStarted = time.Now()
		result.ReaderAnalysis = o.reader.Analyze(result.Candidate)
		result.Steps = append(result.Steps, StepResult{Name: "final_reader_analysis", Status: "completed", DurationMS: time.Since(stepStarted).Milliseconds()})
	}

	result.CandidateWordCount = utf8.RuneCountInString(result.Candidate)
	result.WordCountDeviation = roundQuality(math.Abs(float64(result.CandidateWordCount-result.TargetWordCount)) / float64(result.TargetWordCount))
	stepStarted = time.Now()
	result.QualityGate = buildQualityGate(result)
	result.Steps = append(result.Steps, StepResult{Name: "quality_gate", Status: "completed", DurationMS: time.Since(stepStarted).Milliseconds()})

	if request.ExtractDataChanges && o.dataAgent != nil {
		stepStarted = time.Now()
		result.DataChanges, err = o.dataAgent.ExtractPreview(ctx, request.ChapterNumber, result.Candidate, request.MemoryContext)
		result.Steps = append(result.Steps, stepFromError("data_extract", stepStarted, err))
		if err != nil {
			result.Errors = append(result.Errors, err.Error())
		}
	}

	result.ExecutionTimeMS = time.Since(started).Milliseconds()
	return result, nil
}

func buildQualityGate(result *ChapterWorkflowPreview) QualityGate {
	checks := make([]QualityCheck, 0, 7)
	auditScore := result.Audit.OverallScore
	if result.CandidateAudit != nil {
		auditScore = result.CandidateAudit.OverallScore
	}
	checks = append(checks, QualityCheck{Name: "critic", Passed: result.Passed, Score: auditScore, Message: fmt.Sprintf("Critic %.1f / 阈值 %.1f", auditScore, result.RevisionThreshold)})
	wordCountPassed := result.WordCountDeviation <= 0.35
	checks = append(checks, QualityCheck{Name: "word_count", Passed: wordCountPassed, Score: math.Max(0, 10-result.WordCountDeviation*10), Message: fmt.Sprintf("实际 %d 字 / 目标 %d 字，偏差 %.0f%%", result.CandidateWordCount, result.TargetWordCount, result.WordCountDeviation*100)})
	severeIssues := 0
	for _, issue := range result.ContinuityIssues {
		if issue.Severity == "critical" || issue.Severity == "warning" {
			severeIssues++
		}
	}
	checks = append(checks, QualityCheck{Name: "continuity", Passed: severeIssues == 0, Message: fmt.Sprintf("%d 个严重或警告级问题", severeIssues)})
	readerPassed := result.ReaderAnalysis.AttractivenessScore >= 5
	checks = append(checks, QualityCheck{Name: "reader_attractiveness", Passed: readerPassed, Score: result.ReaderAnalysis.AttractivenessScore, Message: fmt.Sprintf("读者吸引力 %.1f / 建议阈值 5.0", result.ReaderAnalysis.AttractivenessScore)})
	if result.SceneCoverage != nil {
		passed := result.SceneCoverage.CoverageRate >= 0.8 && result.SceneCoverage.OrderValid
		checks = append(checks, QualityCheck{Name: "scene_coverage", Passed: passed, Score: result.SceneCoverage.CoverageRate * 10, Message: fmt.Sprintf("覆盖率 %.0f%%，顺序正常：%t", result.SceneCoverage.CoverageRate*100, result.SceneCoverage.OrderValid)})
	}
	if result.StyleComparison != nil {
		passed := result.StyleComparison.Similarity >= 0.65
		checks = append(checks, QualityCheck{Name: "style_consistency", Passed: passed, Score: result.StyleComparison.Similarity * 10, Message: fmt.Sprintf("与前文相似度 %.0f%%", result.StyleComparison.Similarity*100)})
	}
	if result.AIGCAnalysis != nil {
		passed := result.AIGCAnalysis.RiskLevel != "high"
		checks = append(checks, QualityCheck{Name: "aigc_risk", Passed: passed, Score: result.AIGCAnalysis.OverallScore, Message: "风险等级：" + result.AIGCAnalysis.RiskLevel})
	}
	gate := QualityGate{RecommendedPass: true, Checks: checks, TotalChecks: len(checks), Advisory: true}
	for _, check := range checks {
		if check.Passed {
			gate.PassedChecks++
		} else {
			gate.RecommendedPass = false
		}
	}
	return gate
}

func roundQuality(value float64) float64 {
	return math.Round(value*1000) / 1000
}

func runStep(steps *[]StepResult, name string, action func() (string, error)) (string, error) {
	started := time.Now()
	value, err := action()
	*steps = append(*steps, stepFromError(name, started, err))
	return value, err
}

func stepFromError(name string, started time.Time, err error) StepResult {
	step := StepResult{Name: name, Status: "completed", DurationMS: time.Since(started).Milliseconds()}
	if err != nil {
		step.Status = "failed"
		step.Error = err.Error()
	}
	return step
}

func revisionIssues(audit *agents.AuditResult) []agents.RevisionIssue {
	issues := make([]agents.RevisionIssue, 0, len(audit.Issues)+len(audit.Suggestions))
	for _, issue := range audit.Issues {
		issues = append(issues, agents.RevisionIssue{Problem: issue.Description, Suggestion: "请修复该问题并保持核心剧情不变", Severity: issue.Severity})
	}
	for _, suggestion := range audit.Suggestions {
		issues = append(issues, agents.RevisionIssue{Problem: suggestion, Suggestion: suggestion, Severity: "medium"})
	}
	return issues
}
