package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"novel-agent/internal/service/llm"
)

// Architect Agent - Story outline and structure design
type Architect struct {
	client *llm.Client
}

func NewArchitect(client *llm.Client) *Architect {
	return &Architect{client: client}
}

// ThreeActStructure represents the three-act story structure
type ThreeActStructure struct {
	Act1Setup            string `json:"act1_setup"`
	Act1IncitingIncident string `json:"act1_inciting_incident"`
	Act1PlotPoint1       string `json:"act1_plot_point1"`
	Act2Confrontation    string `json:"act2_confrontation"`
	Act2Midpoint         string `json:"act2_midpoint"`
	Act2PlotPoint2       string `json:"act2_plot_point2"`
	Act3Resolution       string `json:"act3_resolution"`
	Act3Climax           string `json:"act3_climax"`
	Act3Denouement       string `json:"act3_denouement"`
}

// ChapterOutline represents a single chapter outline
type ChapterOutline struct {
	ChapterNumber   int      `json:"chapter_number"`
	Title           string   `json:"title"`
	Summary         string   `json:"summary"`
	KeyEvents       []string `json:"key_events"`
	CharacterArcs   []string `json:"character_arcs"`
	Setting         string   `json:"setting"`
	Conflict        string   `json:"conflict"`
	WordCountTarget int      `json:"word_count_target"`
	NewCharacters   string   `json:"new_characters"`
	OpeningType     string   `json:"opening_type"`
	CliffhangerType string   `json:"cliffhanger_type"`
	POV             string   `json:"pov"`
	Pacing          string   `json:"pacing"`
	BeatOutline     []string `json:"beat_outline"`
	HookPlan        string   `json:"hook_plan"`
	ChapterType     string   `json:"chapter_type"`
}

// GenerateThreeActStructure generates a three-act story structure
func (a *Architect) GenerateThreeActStructure(ctx context.Context, premise, genre string) (*ThreeActStructure, error) {
	prompt := fmt.Sprintf(`你是一位专业的小说架构师。请根据以下故事前提，设计一个完整的三幕式故事结构。

【故事前提】
%s

【题材类型】
%s

请设计一个完整的三幕式故事结构，包含以下部分：

**第一幕：铺垫**
- 故事背景设定
- 主角介绍
- 核心冲突的引入
- 第一转折点

**第二幕：对抗**
- 主要冲突的展开
- 主角的成长与挑战
- 中点转折
- 反派势力的强化
- 第二转折点

**第三幕：解决**
- 高潮对决
- 冲突解决
- 结局收束

请用详细的描述来设计每个部分，以 JSON 格式输出：
{
  "act1_setup": "第一幕详细内容...",
  "act1_inciting_incident": "激励事件...",
  "act1_plot_point1": "第一转折点...",
  "act2_confrontation": "第二幕详细内容...",
  "act2_midpoint": "中点转折...",
  "act2_plot_point2": "第二转折点...",
  "act3_resolution": "第三幕详细内容...",
  "act3_climax": "高潮部分...",
  "act3_denouement": "结局收束..."
}`, premise, genre)

	result, err := a.client.GenerateJSON(ctx, prompt,
		llm.WithTemperature(0.7),
		llm.WithMaxTokens(8000),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to generate three-act structure: %w", err)
	}

	structure, err := decodeThreeActStructure(result)
	if err != nil {
		return nil, fmt.Errorf("invalid three-act structure response: %w", err)
	}
	return structure, nil
}

// GenerateChapterOutline generates an outline for a single chapter
func (a *Architect) GenerateChapterOutline(ctx context.Context, structure *ThreeActStructure, chapterNum, totalChapters int, premise string) (*ChapterOutline, error) {
	if structure == nil {
		return nil, fmt.Errorf("three-act structure is required")
	}
	if totalChapters < 1 || chapterNum < 1 || chapterNum > totalChapters {
		return nil, fmt.Errorf("chapter number must be between 1 and total chapters")
	}
	// Determine which act this chapter belongs to
	var actContent string
	if chapterNum <= totalChapters/3 {
		actContent = structure.Act1Setup
	} else if chapterNum <= totalChapters*2/3 {
		actContent = structure.Act2Confrontation
	} else {
		actContent = structure.Act3Resolution
	}

	prompt := fmt.Sprintf(`你是一位专业的小说大纲设计师。请根据以下信息，为第 %d 章设计详细的大纲。

【全书记事】
%s

【本章属于：%s】
%s

【本章位置】
- 当前章节：第 %d 章
- 总章节数：%d 章
- 进度：%.1f%%

请设计第 %d 章的详细大纲，以 JSON 格式输出：
{
  "title": "章节标题",
  "summary": "本章概要（200字以内）",
  "key_events": ["关键事件1", "关键事件2", "关键事件3"],
  "setting": "本章场景设定",
  "conflict": "本章核心冲突",
  "word_count_target": 5000,
  "opening_type": "开头类型（悬念/动作/对话/描述等）",
  "cliffhanger_type": "结尾钩子类型",
  "chapter_type": "章节类型（战斗章/过渡章/高潮章等）",
  "beat_outline": ["节拍1", "节拍2", "节拍3", "节拍4"],
  "hook_plan": "本章如何设置钩子和伏笔"
}`, chapterNum, premise,
		getActLabel(chapterNum, totalChapters), actContent,
		chapterNum, totalChapters, float64(chapterNum)/float64(totalChapters)*100,
		chapterNum)

	result, err := a.client.GenerateJSON(ctx, prompt,
		llm.WithTemperature(0.6),
		llm.WithMaxTokens(4000),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to generate chapter outline: %w", err)
	}

	outline, err := decodeChapterOutline(result, chapterNum)
	if err != nil {
		return nil, fmt.Errorf("invalid chapter outline response: %w", err)
	}
	return outline, nil
}

// FormatOutline formats a chapter outline as a prompt string
func (a *Architect) FormatOutline(outline *ChapterOutline) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("## 第 %d 章：%s\n\n", outline.ChapterNumber, outline.Title))
	sb.WriteString(fmt.Sprintf("**本章概要：**\n%s\n\n", outline.Summary))
	sb.WriteString(fmt.Sprintf("**场景设定：**\n%s\n\n", outline.Setting))
	sb.WriteString(fmt.Sprintf("**核心冲突：**\n%s\n\n", outline.Conflict))

	if len(outline.KeyEvents) > 0 {
		sb.WriteString("**关键事件：**\n")
		for i, event := range outline.KeyEvents {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, event))
		}
		sb.WriteString("\n")
	}

	if outline.OpeningType != "" {
		sb.WriteString(fmt.Sprintf("**开头类型：**%s\n", outline.OpeningType))
	}
	if outline.CliffhangerType != "" {
		sb.WriteString(fmt.Sprintf("**结尾钩子：**%s\n", outline.CliffhangerType))
	}
	if outline.ChapterType != "" {
		sb.WriteString(fmt.Sprintf("**章节类型：**%s\n", outline.ChapterType))
	}

	if len(outline.BeatOutline) > 0 {
		sb.WriteString("\n**节拍大纲：**\n")
		for i, beat := range outline.BeatOutline {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, beat))
		}
	}

	if outline.HookPlan != "" {
		sb.WriteString(fmt.Sprintf("\n**钩子计划：**\n%s\n", outline.HookPlan))
	}

	sb.WriteString(fmt.Sprintf("\n**目标字数：**%d 字\n", outline.WordCountTarget))

	return sb.String()
}

func requiredNonEmptyString(result map[string]interface{}, key string) (string, error) {
	value, ok := result[key].(string)
	value = strings.TrimSpace(value)
	if !ok || value == "" {
		return "", fmt.Errorf("%s must be a non-empty string", key)
	}
	return value, nil
}

func decodeThreeActStructure(result map[string]interface{}) (*ThreeActStructure, error) {
	keys := []string{"act1_setup", "act1_inciting_incident", "act1_plot_point1", "act2_confrontation", "act2_midpoint", "act2_plot_point2", "act3_resolution", "act3_climax", "act3_denouement"}
	values := make([]string, len(keys))
	for index, key := range keys {
		value, err := requiredNonEmptyString(result, key)
		if err != nil {
			return nil, err
		}
		values[index] = value
	}
	return &ThreeActStructure{Act1Setup: values[0], Act1IncitingIncident: values[1], Act1PlotPoint1: values[2], Act2Confrontation: values[3], Act2Midpoint: values[4], Act2PlotPoint2: values[5], Act3Resolution: values[6], Act3Climax: values[7], Act3Denouement: values[8]}, nil
}

func decodeChapterOutline(result map[string]interface{}, chapterNumber int) (*ChapterOutline, error) {
	keys := []string{"title", "summary", "setting", "conflict", "opening_type", "cliffhanger_type", "chapter_type", "hook_plan"}
	values := make(map[string]string, len(keys))
	for _, key := range keys {
		value, err := requiredNonEmptyString(result, key)
		if err != nil {
			return nil, err
		}
		values[key] = value
	}
	keyEvents, err := requiredStringArray(result, "key_events")
	if err != nil {
		return nil, err
	}
	beats, err := requiredStringArray(result, "beat_outline")
	if err != nil {
		return nil, err
	}
	wordCount, ok := result["word_count_target"].(float64)
	if !ok || math.IsNaN(wordCount) || math.IsInf(wordCount, 0) || wordCount != math.Trunc(wordCount) || wordCount < 100 || wordCount > 20000 {
		return nil, fmt.Errorf("word_count_target must be an integer between 100 and 20000")
	}
	return &ChapterOutline{ChapterNumber: chapterNumber, Title: values["title"], Summary: values["summary"], KeyEvents: keyEvents, Setting: values["setting"], Conflict: values["conflict"], WordCountTarget: int(wordCount), OpeningType: values["opening_type"], CliffhangerType: values["cliffhanger_type"], ChapterType: values["chapter_type"], BeatOutline: beats, HookPlan: values["hook_plan"]}, nil
}

// Helper functions
func getActLabel(chapterNum, total int) string {
	ratio := float64(chapterNum) / float64(total)
	if ratio <= 0.33 {
		return "第一幕"
	} else if ratio <= 0.67 {
		return "第二幕"
	}
	return "第三幕"
}

func getString(m map[string]interface{}, key string) (string, bool) {
	if v, ok := m[key].(string); ok {
		return v, true
	}
	return "", false
}

func getStringArray(m map[string]interface{}, key string) []string {
	if v, ok := m[key].([]interface{}); ok {
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return []string{}
}

// ToJSON converts ThreeActStructure to JSON string
func (t *ThreeActStructure) ToJSON() (string, error) {
	data := map[string]string{
		"act1_setup":             t.Act1Setup,
		"act1_inciting_incident": t.Act1IncitingIncident,
		"act1_plot_point1":       t.Act1PlotPoint1,
		"act2_confrontation":     t.Act2Confrontation,
		"act2_midpoint":          t.Act2Midpoint,
		"act2_plot_point2":       t.Act2PlotPoint2,
		"act3_resolution":        t.Act3Resolution,
		"act3_climax":            t.Act3Climax,
		"act3_denouement":        t.Act3Denouement,
	}

	bytes, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}
