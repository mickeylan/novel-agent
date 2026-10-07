package agents

import (
	"math"
	"strings"
	"unicode/utf8"
)

type EmotionPoint struct {
	Position  float64 `json:"position"`
	Emotion   string  `json:"emotion"`
	Intensity float64 `json:"intensity"`
}

type ReaderIssue struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Suggestion  string `json:"suggestion"`
}

type ReaderPerspectiveResult struct {
	HookStrength        float64        `json:"hook_strength"`
	ClimaxDensity       float64        `json:"climax_density"`
	EmotionCurve        []EmotionPoint `json:"emotion_curve"`
	AttractivenessScore float64        `json:"attractiveness_score"`
	Issues              []ReaderIssue  `json:"issues"`
	Strengths           []string       `json:"strengths"`
}

type ReaderPerspectiveAnalyzer struct{}

func NewReaderPerspectiveAnalyzer() *ReaderPerspectiveAnalyzer {
	return &ReaderPerspectiveAnalyzer{}
}

var climaxKeywords = [][]string{
	{"突破", "进阶", "晋升", "踏入", "境界提升", "修为暴涨"},
	{"逆袭", "反杀", "逆转", "翻身", "逆转乾坤", "形势逆转"},
	{"打脸", "震惊", "愣住", "目瞪口呆", "脸色僵硬", "不敢置信"},
	{"获得", "收获", "得到", "意外之喜", "天降", "意外收获"},
	{"揭秘", "真相", "原来是", "真正身份", "隐藏实力", "暗中布局"},
	{"碾压", "完虐", "秒杀", "轻描淡写", "随手", "轻而易举"},
	{"认主", "归心", "臣服", "追随", "效忠", "投靠"},
	{"贵人", "相助", "出手相助", "暗中保护", "意外援手"},
}

var hookKeywords = [][]string{
	{"悬念", "谜团", "未知", "到底是什么", "背后", "暗藏"},
	{"危机", "危险", "威胁", "逼近", "来临", "将至"},
	{"转折", "突变", "忽然", "意外", "没想到", "竟然"},
	{"期待", "等待", "即将", "很快", "接下来", "就要"},
}

type emotionLexicon struct {
	name     string
	keywords []string
}

var emotionKeywords = []emotionLexicon{
	{"紧张", []string{"紧张", "紧绷", "警惕", "戒备", "战栗", "冷汗"}},
	{"恐惧", []string{"恐惧", "害怕", "惊惧", "骇然", "毛骨悚然", "胆寒"}},
	{"愤怒", []string{"愤怒", "怒火", "恼怒", "愤恨", "怒意", "杀意"}},
	{"惊喜", []string{"惊喜", "欣喜", "振奋", "激动", "雀跃", "喜悦"}},
	{"悲伤", []string{"悲伤", "悲痛", "哀伤", "凄然", "黯然", "落寞"}},
	{"平静", []string{"平静", "冷静", "淡定", "从容", "镇定", "沉稳"}},
	{"期待", []string{"期待", "盼望", "渴望", "向往", "希冀", "憧憬"}},
}

var forbiddenEndings = []string{"才刚刚开始", "叩响新的篇章", "拉开序幕", "踏上征程", "属于他的传奇"}

func (a *ReaderPerspectiveAnalyzer) Analyze(content string) ReaderPerspectiveResult {
	hook := analyzeHooks(content)
	climax := analyzeClimax(content)
	curve := analyzeEmotionCurve(content)
	issues := detectReaderIssues(content, hook, climax, curve)
	strengths := detectReaderStrengths(content, hook, climax)
	return ReaderPerspectiveResult{
		HookStrength:        hook,
		ClimaxDensity:       climax,
		EmotionCurve:        curve,
		AttractivenessScore: calculateAttractiveness(hook, climax, curve),
		Issues:              issues,
		Strengths:           strengths,
	}
}

func analyzeHooks(content string) float64 {
	lines := make([]string, 0)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if utf8.RuneCountInString(line) > 20 {
			lines = append(lines, line)
		}
	}
	if len(lines) < 5 {
		return 0
	}
	count := len(lines) / 10
	if count < 3 {
		count = 3
	}
	opening := strings.Join(lines[:count], " ")
	closing := strings.Join(lines[len(lines)-count:], " ")
	openingScore, closingScore := 0.0, 0.0
	for _, group := range hookKeywords {
		for _, keyword := range group {
			if strings.Contains(opening, keyword) {
				openingScore++
			}
			if strings.Contains(closing, keyword) {
				closingScore++
			}
		}
	}
	if strings.Contains(closing, "……") || strings.Contains(closing, "...") {
		closingScore += 2
	}
	for _, phrase := range forbiddenEndings {
		if strings.Contains(closing, phrase) {
			closingScore -= 5
		}
	}
	return clamp((openingScore+closingScore)/2, 0, 10)
}

func analyzeClimax(content string) float64 {
	characters := utf8.RuneCountInString(content)
	if characters < 100 {
		return 0
	}
	count := 0
	for _, group := range climaxKeywords {
		for _, keyword := range group {
			count += strings.Count(content, keyword)
		}
	}
	density := float64(count) / (float64(characters) / 1000)
	return math.Min(10, density/5*10)
}

func analyzeEmotionCurve(content string) []EmotionPoint {
	runes := []rune(content)
	segmentSize := len(runes) / 10
	if segmentSize < 100 {
		segmentSize = 100
	}
	points := make([]EmotionPoint, 0, 10)
	for index := 0; index < 10; index++ {
		start := index * segmentSize
		end := start + segmentSize
		if start > len(runes) {
			start = len(runes)
		}
		if end > len(runes) {
			end = len(runes)
		}
		emotion, intensity := analyzeEmotionSegment(string(runes[start:end]))
		points = append(points, EmotionPoint{Position: float64(index)/10 + 0.05, Emotion: emotion, Intensity: intensity})
	}
	return points
}

func analyzeEmotionSegment(segment string) (string, float64) {
	bestName, bestScore := "平静", 0
	for _, lexicon := range emotionKeywords {
		score := 0
		for _, keyword := range lexicon.keywords {
			score += strings.Count(segment, keyword)
		}
		if score > bestScore {
			bestName, bestScore = lexicon.name, score
		}
	}
	if bestScore == 0 {
		return "平静", 0.3
	}
	return bestName, math.Min(1, float64(bestScore)/5)
}

func detectReaderIssues(content string, hook, climax float64, curve []EmotionPoint) []ReaderIssue {
	issues := make([]ReaderIssue, 0)
	if hook < 3 {
		issues = append(issues, ReaderIssue{Type: "weak_hook", Description: "钩子强度不足", Suggestion: "增加开头悬念或结尾危机感"})
	}
	if climax < 2 {
		issues = append(issues, ReaderIssue{Type: "low_climax", Description: "爽点密度偏低", Suggestion: "增加突破、逆袭或关键揭示"})
	}
	average := 0.0
	for _, point := range curve {
		average += point.Intensity
	}
	if len(curve) > 0 {
		average /= float64(len(curve))
	}
	if average < 0.3 {
		issues = append(issues, ReaderIssue{Type: "flat_emotion", Description: "情绪曲线过于平淡", Suggestion: "增加情绪起伏"})
	}
	ending := lastRunes(content, 200)
	for _, phrase := range forbiddenEndings[:4] {
		if strings.Contains(ending, phrase) {
			issues = append(issues, ReaderIssue{Type: "forbidden_ending", Description: "发现套话结尾：" + phrase, Suggestion: "改用具体动作、场景或未解决冲突结尾"})
		}
	}
	return issues
}

func detectReaderStrengths(content string, hook, climax float64) []string {
	strengths := make([]string, 0)
	if hook >= 7 {
		strengths = append(strengths, "开头/结尾钩子强劲")
	}
	if climax >= 5 {
		strengths = append(strengths, "爽点密度充足，节奏紧凑")
	}
	sensory := 0
	for _, word := range []string{"触", "听", "看", "闻", "尝", "冷", "热", "痛", "痒", "刺"} {
		sensory += strings.Count(content, word)
	}
	if sensory >= 20 {
		strengths = append(strengths, "感官描写丰富，代入感强")
	}
	return strengths
}

func calculateAttractiveness(hook, climax float64, curve []EmotionPoint) float64 {
	variance := 0.0
	for i := 1; i < len(curve); i++ {
		variance += math.Abs(curve[i].Intensity - curve[i-1].Intensity)
	}
	if len(curve) > 0 {
		variance /= float64(len(curve))
	}
	emotionScore := math.Min(10, variance*20)
	return math.Round((0.3*hook+0.3*climax+0.4*emotionScore)*10) / 10
}

func lastRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) > limit {
		runes = runes[len(runes)-limit:]
	}
	return string(runes)
}

func clamp(value, minimum, maximum float64) float64 {
	return math.Max(minimum, math.Min(maximum, value))
}
