package agents

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type ContinuityIssue struct {
	Severity   string `json:"severity"`
	Category   string `json:"category"`
	Message    string `json:"message"`
	Location   string `json:"location"`
	Suggestion string `json:"suggestion"`
}

type ContinuityValidator struct{}

func NewContinuityValidator() *ContinuityValidator { return &ContinuityValidator{} }

func (v *ContinuityValidator) ValidateChapter(current, previous string, newCharacters []string) []ContinuityIssue {
	issues := make([]ContinuityIssue, 0)
	issues = append(issues, v.checkCharacterEntries(current, newCharacters)...)
	if previous != "" {
		issues = append(issues, v.checkOpening(current, previous)...)
		if issue := v.checkTransition(current); issue != nil {
			issues = append(issues, *issue)
		}
	}
	issues = append(issues, v.checkLogic(current)...)
	order := map[string]int{"critical": 0, "warning": 1, "info": 2}
	sort.SliceStable(issues, func(i, j int) bool { return order[issues[i].Severity] < order[issues[j].Severity] })
	return issues
}

func (v *ContinuityValidator) Summary(issues []ContinuityIssue) string {
	if len(issues) == 0 {
		return "✅ 连贯性检查通过！"
	}
	counts := map[string]int{}
	for _, issue := range issues {
		counts[issue.Severity]++
	}
	parts := make([]string, 0, 3)
	if counts["critical"] > 0 {
		parts = append(parts, fmt.Sprintf("❌ %d 个严重问题", counts["critical"]))
	}
	if counts["warning"] > 0 {
		parts = append(parts, fmt.Sprintf("⚠️ %d 个警告", counts["warning"]))
	}
	if counts["info"] > 0 {
		parts = append(parts, fmt.Sprintf("ℹ️ %d 个提示", counts["info"]))
	}
	return strings.Join(parts, " | ")
}

func (v *ContinuityValidator) checkCharacterEntries(content string, names []string) []ContinuityIssue {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`传来|响起|走出|出现|浮现|忽听|只见|闻到|感觉到|察觉到`),
		regexp.MustCompile(`脚步声|气息|动静|声音|身影|缓缓|慢慢|由远及近|从.*[中里后]`),
	}
	issues := make([]ContinuityIssue, 0)
	runes := []rune(content)
	for _, name := range names {
		index := strings.Index(content, name)
		if index < 0 {
			continue
		}
		prefixRunes := []rune(content[:index])
		start := len(prefixRunes) - 200
		if start < 0 {
			start = 0
		}
		before := string(runes[start:len(prefixRunes)])
		hasTransition := false
		for _, pattern := range patterns {
			if pattern.MatchString(before) {
				hasTransition = true
				break
			}
		}
		if !hasTransition {
			issues = append(issues, ContinuityIssue{Severity: "warning", Category: "character_entry", Message: fmt.Sprintf("人物'%s'出场缺少自然过渡", name), Location: fmt.Sprintf("第%d段附近", strings.Count(content[:index], "\n\n")+1), Suggestion: "建议在人物首次出现前增加声音、气息、动作或环境变化铺垫"})
		}
	}
	return issues
}

func (v *ContinuityValidator) checkOpening(current, previous string) []ContinuityIssue {
	currentOpening := firstRunes(current, 300)
	previousOpening := firstRunes(previous, 300)
	issues := make([]ContinuityIssue, 0, 2)
	currentType, previousType := classifyOpening(currentOpening), classifyOpening(previousOpening)
	if currentType == previousType {
		issues = append(issues, ContinuityIssue{Severity: "warning", Category: "opening_repeat", Message: "本章开头与前章开头类型重复（都是'" + currentType + "'）", Location: "开头第1段", Suggestion: "建议改用不同的对话、动作、环境或心理开头"})
	}
	if similarity := runeSetSimilarity(currentOpening, previousOpening); similarity > 0.6 {
		issues = append(issues, ContinuityIssue{Severity: "info", Category: "opening_repeat", Message: fmt.Sprintf("本章开头与前章开头相似度较高（%.0f%%）", similarity*100), Location: "开头第1-2段", Suggestion: "建议调整句式结构，避免重复"})
	}
	return issues
}

func (v *ContinuityValidator) checkTransition(content string) *ContinuityIssue {
	first := firstRunes(content, 200)
	if parts := strings.SplitN(content, "\n\n", 2); len(parts) > 1 {
		first = parts[0]
	}
	if regexp.MustCompile(`不远处.*站着|突然.*出现|这时.*走来|只见.*站在`).MatchString(first) {
		return &ContinuityIssue{Severity: "warning", Category: "transition", Message: "章节开头存在硬切，情节衔接不够自然", Location: "开头第1段", Suggestion: "建议增加时间、空间或感知过渡"}
	}
	return nil
}

func (v *ContinuityValidator) checkLogic(content string) []ContinuityIssue {
	issues := make([]ContinuityIssue, 0, 2)
	times := make([]string, 0)
	for _, word := range []string{"清晨", "正午", "黄昏", "夜晚", "深夜", "凌晨"} {
		if strings.Contains(content, word) {
			times = append(times, word)
		}
	}
	if len(times) > 2 && !strings.Contains(content, "片刻") && !strings.Contains(content, "不多时") && !strings.Contains(content, "过了") {
		issues = append(issues, ContinuityIssue{Severity: "info", Category: "logic", Message: "章节内时间跳跃较快（" + strings.Join(times, "、") + "）", Location: "全文", Suggestion: "建议添加时间流逝描写"})
	}
	if strings.Contains(content, "我") && strings.Contains(content, "他") {
		issues = append(issues, ContinuityIssue{Severity: "info", Category: "logic", Message: "检测到第一人称和第三人称混用，请确认叙事视角", Location: "全文", Suggestion: "检查并统一叙事视角"})
	}
	return issues
}

func classifyOpening(opening string) string {
	opening = strings.TrimSpace(opening)
	if strings.HasPrefix(opening, `"`) || strings.HasPrefix(opening, `'`) || strings.HasPrefix(opening, `“`) {
		return "对话开头"
	}
	for _, prefix := range []string{"雨", "夜", "清晨", "阳光", "风", "天", "月"} {
		if strings.HasPrefix(opening, prefix) {
			return "环境描写开头"
		}
	}
	if (strings.HasPrefix(opening, "他") || strings.HasPrefix(opening, "她")) && strings.ContainsAny(opening, "站走跑看听握抬") {
		return "动作开头"
	}
	for _, word := range []string{"想", "觉得", "感到", "心中", "暗道"} {
		if strings.Contains(opening, word) {
			return "心理活动开头"
		}
	}
	return "叙事开头"
}

func runeSetSimilarity(a, b string) float64 {
	left, right := map[rune]struct{}{}, map[rune]struct{}{}
	for _, r := range a {
		if !isSpace(r) {
			left[r] = struct{}{}
		}
	}
	for _, r := range b {
		if !isSpace(r) {
			right[r] = struct{}{}
		}
	}
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	intersection, union := 0, len(left)
	for r := range right {
		if _, ok := left[r]; ok {
			intersection++
		} else {
			union++
		}
	}
	return float64(intersection) / float64(union)
}

func firstRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) > limit {
		runes = runes[:limit]
	}
	return string(runes)
}

func isSpace(r rune) bool { return r == ' ' || r == '\n' || r == '\r' || r == '\t' }
