package agents

import (
	"fmt"
	"math"
	"strings"
	"unicode"
)

type ScenePlan struct {
	Index           int    `json:"index"`
	Purpose         string `json:"purpose"`
	EntryHook       string `json:"entry_hook"`
	Development     string `json:"development"`
	ExitHook        string `json:"exit_hook"`
	TargetWordCount int    `json:"target_word_count"`
}

type ScenePlanPreview struct {
	Scenes          []ScenePlan `json:"scenes"`
	SceneCount      int         `json:"scene_count"`
	TargetWordCount int         `json:"target_word_count"`
	Source          string      `json:"source"`
	Truncated       bool        `json:"truncated"`
	Preview         bool        `json:"preview"`
}

type SceneCoverage struct {
	SceneIndex      int      `json:"scene_index"`
	Purpose         string   `json:"purpose"`
	Covered         bool     `json:"covered"`
	CoverageScore   float64  `json:"coverage_score"`
	MatchedTerms    []string `json:"matched_terms"`
	FirstPosition   int      `json:"first_position"`
	EstimatedRunes  int      `json:"estimated_runes"`
	TargetWordCount int      `json:"target_word_count"`
	LengthDeviation float64  `json:"length_deviation"`
	LengthWarning   bool     `json:"length_warning"`
}

type SceneCoverageResult struct {
	CoverageRate    float64         `json:"coverage_rate"`
	CoveredScenes   int             `json:"covered_scenes"`
	TotalScenes     int             `json:"total_scenes"`
	OrderValid      bool            `json:"order_valid"`
	OrderViolations []int           `json:"order_violations"`
	Scenes          []SceneCoverage `json:"scenes"`
	ContentRunes    int             `json:"content_runes"`
	Preview         bool            `json:"preview"`
}

type ScenePlanner struct{}

func NewScenePlanner() *ScenePlanner { return &ScenePlanner{} }

func (p *ScenePlanner) Plan(outline string, keyEvents []string, targetWordCount, maxScenes int) ScenePlanPreview {
	if targetWordCount < 100 {
		targetWordCount = 5000
	}
	if maxScenes < 1 || maxScenes > 10 {
		maxScenes = 8
	}
	events := normalizeSceneEvents(keyEvents)
	source := "key_events"
	if len(events) == 0 {
		events = outlineEvents(outline)
		source = "outline"
	}
	if len(events) == 0 {
		events = []string{"根据章节大纲推进核心情节"}
		source = "fallback"
	}
	truncated := len(events) > maxScenes
	if truncated {
		events = events[:maxScenes]
	}
	allocations := allocateWords(targetWordCount, len(events))
	scenes := make([]ScenePlan, len(events))
	for index, event := range events {
		scenes[index] = ScenePlan{
			Index: index + 1, Purpose: event,
			EntryHook:       sceneEntry(index, event),
			Development:     "围绕“" + event + "”推进动作、对话和角色选择，并形成可观察的变化。",
			ExitHook:        sceneExit(index, len(events)),
			TargetWordCount: allocations[index],
		}
	}
	return ScenePlanPreview{Scenes: scenes, SceneCount: len(scenes), TargetWordCount: targetWordCount, Source: source, Truncated: truncated, Preview: true}
}

func (p *ScenePlanner) FormatForPrompt(plan ScenePlanPreview) string {
	if len(plan.Scenes) == 0 {
		return ""
	}
	var builder strings.Builder
	builder.WriteString("【场景执行计划】\n")
	builder.WriteString("按以下顺序推进，但允许自然过渡；不要输出场景编号或计划说明。\n")
	for _, scene := range plan.Scenes {
		builder.WriteString(fmt.Sprintf("%d. %s（约%d字）\n", scene.Index, scene.Purpose, scene.TargetWordCount))
		builder.WriteString("   切入：" + scene.EntryHook + "\n")
		builder.WriteString("   推进：" + scene.Development + "\n")
		builder.WriteString("   转出：" + scene.ExitHook + "\n")
	}
	return strings.TrimSpace(builder.String())
}

func (p *ScenePlanner) ValidateCoverage(content string, plan ScenePlanPreview) SceneCoverageResult {
	content = strings.TrimSpace(content)
	contentRunes := []rune(content)
	result := SceneCoverageResult{TotalScenes: len(plan.Scenes), OrderValid: true, ContentRunes: len(contentRunes), Preview: true, Scenes: make([]SceneCoverage, len(plan.Scenes)), OrderViolations: []int{}}
	previousPosition := -1
	for index, scene := range plan.Scenes {
		terms := sceneTerms(scene.Purpose)
		matched := make([]string, 0, len(terms))
		firstPosition := -1
		for _, term := range terms {
			bytePosition := strings.Index(content, term)
			if bytePosition < 0 {
				continue
			}
			matched = append(matched, term)
			position := len([]rune(content[:bytePosition]))
			if firstPosition < 0 || position < firstPosition {
				firstPosition = position
			}
		}
		score := 0.0
		if len(terms) > 0 {
			score = float64(len(matched)) / float64(len(terms))
		}
		covered := score >= 0.5
		if covered {
			result.CoveredScenes++
			if previousPosition >= 0 && firstPosition < previousPosition {
				result.OrderValid = false
				result.OrderViolations = append(result.OrderViolations, scene.Index)
			}
			previousPosition = firstPosition
		}
		result.Scenes[index] = SceneCoverage{SceneIndex: scene.Index, Purpose: scene.Purpose, Covered: covered, CoverageScore: round3(score), MatchedTerms: matched, FirstPosition: firstPosition, TargetWordCount: scene.TargetWordCount}
	}
	if result.TotalScenes > 0 {
		result.CoverageRate = round3(float64(result.CoveredScenes) / float64(result.TotalScenes))
	}
	for index := range result.Scenes {
		if !result.Scenes[index].Covered {
			continue
		}
		start := result.Scenes[index].FirstPosition
		end := len(contentRunes)
		for next := index + 1; next < len(result.Scenes); next++ {
			if result.Scenes[next].Covered && result.Scenes[next].FirstPosition > start {
				end = result.Scenes[next].FirstPosition
				break
			}
		}
		actual := end - start
		result.Scenes[index].EstimatedRunes = actual
		if result.Scenes[index].TargetWordCount > 0 {
			deviation := math.Abs(float64(actual-result.Scenes[index].TargetWordCount)) / float64(result.Scenes[index].TargetWordCount)
			result.Scenes[index].LengthDeviation = round3(deviation)
			result.Scenes[index].LengthWarning = deviation > 0.6
		}
	}
	return result
}

func sceneTerms(purpose string) []string {
	parts := strings.FieldsFunc(purpose, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune("，。；：、,.!?！？（）()“”\"'", r)
	})
	terms := make([]string, 0)
	seen := map[string]struct{}{}
	for _, part := range parts {
		runes := []rune(strings.TrimSpace(part))
		if len(runes) < 2 {
			continue
		}
		if len(runes) <= 6 {
			if _, exists := seen[string(runes)]; !exists {
				seen[string(runes)] = struct{}{}
				terms = append(terms, string(runes))
			}
			continue
		}
		for index := 0; index+1 < len(runes); index += 2 {
			end := index + 2
			if end > len(runes) {
				end = len(runes)
			}
			term := string(runes[index:end])
			if _, exists := seen[term]; !exists {
				seen[term] = struct{}{}
				terms = append(terms, term)
			}
		}
	}
	return terms
}

func normalizeSceneEvents(events []string) []string {
	result := make([]string, 0, len(events))
	seen := map[string]struct{}{}
	for _, event := range events {
		event = strings.TrimSpace(event)
		if event == "" {
			continue
		}
		if _, exists := seen[event]; exists {
			continue
		}
		seen[event] = struct{}{}
		result = append(result, event)
	}
	return result
}

func outlineEvents(outline string) []string {
	parts := strings.FieldsFunc(strings.ReplaceAll(outline, "\r\n", "\n"), func(r rune) bool {
		return r == '\n' || r == '。' || r == '；' || r == ';'
	})
	return normalizeSceneEvents(parts)
}

func allocateWords(total, count int) []int {
	if count <= 0 {
		return nil
	}
	result := make([]int, count)
	base, remainder := total/count, total%count
	for index := range result {
		result[index] = base
		if index < remainder {
			result[index]++
		}
	}
	return result
}

func sceneEntry(index int, event string) string {
	if index == 0 {
		return "用具体动作、异常或冲突切入：“" + event + "”。"
	}
	return fmt.Sprintf("承接上一场景的后果，立即进入事件：“%s”。", event)
}

func sceneExit(index, total int) string {
	if index == total-1 {
		return "以未解决的问题、代价或新信息收束，形成章节钩子。"
	}
	return "让角色的选择产生后果，并把因果链交给下一场景。"
}
