package agents

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"

	"novel-agent/internal/models"
)

type TermFrequency struct {
	Term  string `json:"term"`
	Count int    `json:"count"`
}

type StyleProfile struct {
	AverageSentenceLength  float64         `json:"average_sentence_length"`
	AverageParagraphLength float64         `json:"average_paragraph_length"`
	VocabularyRichness     float64         `json:"vocabulary_richness"`
	DialogueRatio          float64         `json:"dialogue_ratio"`
	SentenceLengthVariance float64         `json:"sentence_length_variance"`
	CommonConnectives      []TermFrequency `json:"common_connectives"`
	PunctuationStyle       map[string]int  `json:"punctuation_style"`
	SentenceCount          int             `json:"sentence_count"`
	ParagraphCount         int             `json:"paragraph_count"`
	CharacterCount         int             `json:"character_count"`
	AnalysisVersion        string          `json:"analysis_version"`
	Preview                bool            `json:"preview"`
}

type StyleComparison struct {
	Current                 StyleProfile `json:"current"`
	Reference               StyleProfile `json:"reference"`
	Similarity              float64      `json:"similarity"`
	ReferenceChapterNumbers []int        `json:"reference_chapter_numbers"`
	ReferenceTruncated      bool         `json:"reference_truncated"`
	Preview                 bool         `json:"preview"`
}

type StyleAnalyzer struct{}

func NewStyleAnalyzer() *StyleAnalyzer { return &StyleAnalyzer{} }

var styleConnectives = []string{
	"然而", "但是", "可是", "不过", "因此", "所以", "于是", "虽然", "尽管", "如果",
	"因为", "首先", "其次", "然后", "接着", "随后", "最后", "同时", "另外", "此外",
	"突然", "忽然", "渐渐", "慢慢", "竟然", "居然", "果然", "确实",
}

var trackedPunctuation = []rune{'。', '，', '！', '？', '；', '：', '…', '—', '“', '”'}

func (a *StyleAnalyzer) CompareWithPrevious(content string, chapters []models.Chapter, beforeChapter, maxSamples, maxRunes int) StyleComparison {
	if maxSamples < 1 || maxSamples > 5 {
		maxSamples = 5
	}
	if maxRunes < 1000 || maxRunes > 50000 {
		maxRunes = 50000
	}
	ordered := append([]models.Chapter(nil), chapters...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].ChapterNumber < ordered[j].ChapterNumber })
	selected := make([]models.Chapter, 0, maxSamples)
	for index := len(ordered) - 1; index >= 0 && len(selected) < maxSamples; index-- {
		chapter := ordered[index]
		if chapter.ChapterNumber >= beforeChapter || chapter.Content == nil || strings.TrimSpace(*chapter.Content) == "" {
			continue
		}
		selected = append([]models.Chapter{chapter}, selected...)
	}
	var reference strings.Builder
	numbers := make([]int, 0, len(selected))
	for _, chapter := range selected {
		if reference.Len() > 0 {
			reference.WriteString("\n")
		}
		reference.WriteString(strings.TrimSpace(*chapter.Content))
		numbers = append(numbers, chapter.ChapterNumber)
	}
	referenceRunes := []rune(reference.String())
	truncated := len(referenceRunes) > maxRunes
	if truncated {
		referenceRunes = referenceRunes[len(referenceRunes)-maxRunes:]
	}
	currentProfile := a.Analyze(content)
	referenceProfile := a.Analyze(string(referenceRunes))
	similarity := 0.0
	if referenceProfile.CharacterCount > 0 {
		similarity = styleSimilarity(currentProfile, referenceProfile)
	}
	return StyleComparison{Current: currentProfile, Reference: referenceProfile, Similarity: similarity, ReferenceChapterNumbers: numbers, ReferenceTruncated: truncated, Preview: true}
}

func (a *StyleAnalyzer) CompareWithProfile(content string, reference StyleProfile, chapterNumbers []int, referenceTruncated bool) StyleComparison {
	current := a.Analyze(content)
	similarity := 0.0
	if reference.CharacterCount > 0 {
		similarity = styleSimilarity(current, reference)
	}
	return StyleComparison{
		Current: current, Reference: reference, Similarity: similarity,
		ReferenceChapterNumbers: append([]int(nil), chapterNumbers...),
		ReferenceTruncated:      referenceTruncated, Preview: true,
	}
}

func (a *StyleAnalyzer) BuildGuide(profile StyleProfile) string {
	if profile.CharacterCount == 0 {
		return ""
	}
	var guide strings.Builder
	guide.WriteString("保持与前文章节一致的文风：\n")
	guide.WriteString(fmt.Sprintf("- 句子平均长度约 %.0f 字，长短句需要自然变化。\n", profile.AverageSentenceLength))
	guide.WriteString(fmt.Sprintf("- 段落平均长度约 %.0f 字，避免机械等长分段。\n", profile.AverageParagraphLength))
	guide.WriteString(fmt.Sprintf("- 对话占比约 %.0f%%，保持叙述与对话比例接近。\n", profile.DialogueRatio*100))
	if len(profile.CommonConnectives) > 0 {
		terms := make([]string, 0, 5)
		for index, item := range profile.CommonConnectives {
			if index >= 5 {
				break
			}
			terms = append(terms, item.Term)
		}
		guide.WriteString("- 前文常用连接词包括：" + strings.Join(terms, "、") + "；自然使用，不要集中堆叠。\n")
	}
	guide.WriteString("- 保持角色口吻与具体感官细节，不要复制前文原句。")
	return guide.String()
}

func (a *StyleAnalyzer) Analyze(content string) StyleProfile {
	content = strings.TrimSpace(content)
	sentences := splitSentences(content)
	paragraphs := nonEmptyParts(strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n"))
	sentenceLengths := runeLengths(sentences)
	paragraphLengths := runeLengths(paragraphs)
	letters, uniqueLetters := letterCounts(content)
	characterCount := len([]rune(content))

	profile := StyleProfile{
		AverageSentenceLength:  round2(average(sentenceLengths)),
		AverageParagraphLength: round2(average(paragraphLengths)),
		SentenceLengthVariance: round2(variance(sentenceLengths)),
		DialogueRatio:          round3(dialogueRatio(content, characterCount)),
		CommonConnectives:      connectiveFrequencies(content),
		PunctuationStyle:       punctuationFrequencies(content),
		SentenceCount:          len(sentences), ParagraphCount: len(paragraphs), CharacterCount: characterCount,
		AnalysisVersion: "style-heuristic-v1", Preview: true,
	}
	if letters > 0 {
		profile.VocabularyRichness = round3(float64(uniqueLetters) / float64(letters))
	}
	return profile
}

func runeLengths(values []string) []float64 {
	result := make([]float64, len(values))
	for i, value := range values {
		result[i] = float64(len([]rune(value)))
	}
	return result
}

func letterCounts(content string) (int, int) {
	letters := 0
	unique := map[rune]struct{}{}
	for _, r := range content {
		if unicode.IsLetter(r) {
			letters++
			unique[r] = struct{}{}
		}
	}
	return letters, len(unique)
}

func dialogueRatio(content string, characterCount int) float64 {
	if characterCount == 0 {
		return 0
	}
	inDialogue := false
	dialogueRunes := 0
	for _, r := range content {
		switch r {
		case '“':
			inDialogue = true
		case '”':
			inDialogue = false
		default:
			if inDialogue {
				dialogueRunes++
			}
		}
	}
	return clampAIGC(float64(dialogueRunes)/float64(characterCount), 0, 1)
}

func connectiveFrequencies(content string) []TermFrequency {
	result := make([]TermFrequency, 0)
	for _, term := range styleConnectives {
		if count := strings.Count(content, term); count > 0 {
			result = append(result, TermFrequency{Term: term, Count: count})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Count == result[j].Count {
			return result[i].Term < result[j].Term
		}
		return result[i].Count > result[j].Count
	})
	if len(result) > 10 {
		result = result[:10]
	}
	return result
}

func punctuationFrequencies(content string) map[string]int {
	result := make(map[string]int, len(trackedPunctuation))
	for _, punctuation := range trackedPunctuation {
		result[string(punctuation)] = 0
	}
	for _, r := range content {
		if _, tracked := result[string(r)]; tracked {
			result[string(r)]++
		}
	}
	return result
}

func styleSimilarity(left, right StyleProfile) float64 {
	differences := []float64{
		normalizedDifference(left.AverageSentenceLength, right.AverageSentenceLength, 50),
		normalizedDifference(left.AverageParagraphLength, right.AverageParagraphLength, 500),
		math.Abs(left.VocabularyRichness - right.VocabularyRichness),
		math.Abs(left.DialogueRatio - right.DialogueRatio),
	}
	return round3(clampAIGC(1-average(differences), 0, 1))
}

func normalizedDifference(left, right, scale float64) float64 {
	if scale <= 0 {
		return 0
	}
	return clampAIGC(math.Abs(left-right)/scale, 0, 1)
}
