package agents

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

type AIGCPattern struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
	Count       int    `json:"count"`
}

type AIGCMetrics struct {
	SentenceLengthVariance float64 `json:"sentence_length_variance"`
	StructureDiversity     float64 `json:"structure_diversity"`
	ParagraphVariance      float64 `json:"paragraph_variance"`
	TransitionModeration   float64 `json:"transition_moderation"`
	SpecificDetails        float64 `json:"specific_details"`
	EmotionalExpression    float64 `json:"emotional_expression"`
}

type AIGCDetectionResult struct {
	RiskLevel       string        `json:"risk_level"`
	OverallScore    float64       `json:"overall_score"`
	Patterns        []AIGCPattern `json:"patterns_detected"`
	Suggestions     []string      `json:"suggestions"`
	Metrics         AIGCMetrics   `json:"metrics"`
	SentenceCount   int           `json:"sentence_count"`
	CharacterCount  int           `json:"character_count"`
	AnalysisVersion string        `json:"analysis_version"`
	Preview         bool          `json:"preview"`
}

type AIGCDetector struct{}

func NewAIGCDetector() *AIGCDetector { return &AIGCDetector{} }

var aiTransitions = []string{"然而", "因此", "此外", "同时", "不过", "但是", "总之", "综上所述"}
var emotionalIndicators = []string{"！", "啊", "呀", "呢", "吧", "唉", "哼", "哈"}

func (d *AIGCDetector) Detect(content string) AIGCDetectionResult {
	sentences := splitSentences(content)
	metrics := analyzeAIGCMetrics(content, sentences)
	patterns := detectAIGCPatterns(content, sentences)
	humanScore := average([]float64{metrics.SentenceLengthVariance, metrics.StructureDiversity, metrics.ParagraphVariance, metrics.TransitionModeration, metrics.SpecificDetails, metrics.EmotionalExpression}) * 10
	humanScore = round2(clampAIGC(humanScore, 0, 10))
	risk := "low"
	if humanScore < 4 {
		risk = "high"
	} else if humanScore < 7 {
		risk = "medium"
	}
	return AIGCDetectionResult{
		RiskLevel: risk, OverallScore: humanScore, Patterns: patterns,
		Suggestions: aigcSuggestions(metrics), Metrics: metrics,
		SentenceCount: len(sentences), CharacterCount: len([]rune(strings.TrimSpace(content))),
		AnalysisVersion: "heuristic-v1", Preview: true,
	}
}

func analyzeAIGCMetrics(content string, sentences []string) AIGCMetrics {
	if len(sentences) == 0 {
		return AIGCMetrics{}
	}
	lengths := make([]float64, len(sentences))
	starters := map[string]struct{}{}
	for i, sentence := range sentences {
		runes := []rune(sentence)
		lengths[i] = float64(len(runes))
		if len(runes) > 2 {
			starters[string(runes[:2])] = struct{}{}
		} else if len(runes) > 0 {
			starters[string(runes)] = struct{}{}
		}
	}
	paragraphs := nonEmptyParts(strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n"))
	paragraphLengths := make([]float64, len(paragraphs))
	for i, paragraph := range paragraphs {
		paragraphLengths[i] = float64(len([]rune(paragraph)))
	}
	transitionCount := countTerms(content, aiTransitions)
	emotionCount := countTerms(content, emotionalIndicators)
	detailCount := countDetails(content)
	chars := math.Max(float64(len([]rune(content))), 1)
	return AIGCMetrics{
		SentenceLengthVariance: round3(clampAIGC(variance(lengths)/1000, 0, 1)),
		StructureDiversity:     round3(clampAIGC(float64(len(starters))/float64(len(sentences)), 0, 1)),
		ParagraphVariance:      round3(clampAIGC(variance(paragraphLengths)/5000, 0, 1)),
		TransitionModeration:   round3(clampAIGC(1-float64(transitionCount)/float64(len(sentences)), 0, 1)),
		SpecificDetails:        round3(clampAIGC(float64(detailCount)/(chars/100), 0, 1)),
		EmotionalExpression:    round3(clampAIGC(float64(emotionCount)/(float64(len(sentences))/2), 0, 1)),
	}
}

func detectAIGCPatterns(content string, sentences []string) []AIGCPattern {
	patterns := make([]AIGCPattern, 0)
	starters := map[string]int{}
	for _, sentence := range sentences {
		runes := []rune(sentence)
		if len(runes) >= 2 {
			starters[string(runes[:2])]++
		}
	}
	starterKeys := make([]string, 0, len(starters))
	for starter := range starters {
		starterKeys = append(starterKeys, starter)
	}
	sort.Strings(starterKeys)
	for _, starter := range starterKeys {
		if count := starters[starter]; count > 3 {
			patterns = append(patterns, AIGCPattern{Type: "repetitive_starter", Description: "句子重复以“" + starter + "”开头", Severity: "medium", Count: count})
		}
	}
	for _, transition := range aiTransitions {
		if count := strings.Count(content, transition); count > 5 {
			patterns = append(patterns, AIGCPattern{Type: "transition_overuse", Description: "过度使用过渡词“" + transition + "”", Severity: "low", Count: count})
		}
	}
	return patterns
}

func aigcSuggestions(metrics AIGCMetrics) []string {
	result := make([]string, 0, 5)
	if metrics.SentenceLengthVariance < .3 {
		result = append(result, "增加句式长度变化，混合使用长短句")
	}
	if metrics.StructureDiversity < .5 {
		result = append(result, "丰富句子开头，避免连续使用相同句式")
	}
	if metrics.SpecificDetails < .4 {
		result = append(result, "补充具体的人名、地名、时间、数量或感官细节")
	}
	if metrics.EmotionalExpression < .3 {
		result = append(result, "通过动作、对话和内心反应增强情绪波动")
	}
	if metrics.TransitionModeration < .5 {
		result = append(result, "减少“然而、因此、此外”等显式过渡词")
	}
	if len(result) < 3 {
		result = append(result, "增加带有角色个性的对话与动作细节")
	}
	if len(result) > 5 {
		result = result[:5]
	}
	return result
}

func splitSentences(content string) []string {
	return nonEmptyParts(strings.FieldsFunc(content, func(r rune) bool { return strings.ContainsRune(".!?。！？", r) }))
}
func nonEmptyParts(parts []string) []string {
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}
func variance(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}
	mean := average(values)
	total := 0.0
	for _, v := range values {
		total += (v - mean) * (v - mean)
	}
	return total / float64(len(values))
}
func average(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	total := 0.0
	for _, v := range values {
		total += v
	}
	return total / float64(len(values))
}
func countTerms(content string, terms []string) int {
	total := 0
	for _, term := range terms {
		total += strings.Count(content, term)
	}
	return total
}
func countDetails(content string) int {
	count := 0
	inDigits := false
	for _, r := range content {
		if unicode.IsDigit(r) {
			if !inDigits {
				count++
			}
			inDigits = true
		} else {
			inDigits = false
		}
	}
	return count
}
func clampAIGC(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
func round2(value float64) float64 { return math.Round(value*100) / 100 }
func round3(value float64) float64 { return math.Round(value*1000) / 1000 }
