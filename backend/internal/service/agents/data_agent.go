package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"novel-agent/internal/service/llm"
)

type StateChange struct {
	EntityType string  `json:"entity_type"`
	EntityName string  `json:"entity_name"`
	Field      string  `json:"field"`
	OldValue   string  `json:"old_value,omitempty"`
	NewValue   string  `json:"new_value"`
	Evidence   string  `json:"evidence"`
	Confidence float64 `json:"confidence"`
}

type NewEntity struct {
	EntityType  string            `json:"entity_type"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Attributes  map[string]string `json:"attributes"`
	Evidence    string            `json:"evidence"`
	Confidence  float64           `json:"confidence"`
}

type ResourceChange struct {
	CharacterName string  `json:"character_name"`
	ResourceType  string  `json:"resource_type"`
	ResourceName  string  `json:"resource_name"`
	PreviousValue string  `json:"previous_value,omitempty"`
	CurrentValue  string  `json:"current_value"`
	Evidence      string  `json:"evidence"`
	Confidence    float64 `json:"confidence"`
}

type HookCandidate struct {
	HookType                  string   `json:"hook_type"`
	Description               string   `json:"description"`
	Priority                  string   `json:"priority"`
	ExpectedResolutionChapter *int     `json:"expected_resolution_chapter,omitempty"`
	RelatedCharacters         []string `json:"related_characters"`
	Evidence                  string   `json:"evidence"`
	Confidence                float64  `json:"confidence"`
}

type SubplotChange struct {
	Name        string   `json:"name"`
	ChangeType  string   `json:"change_type"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	Characters  []string `json:"characters"`
	Evidence    string   `json:"evidence"`
	Confidence  float64  `json:"confidence"`
}

type DataExtractionPreview struct {
	StateChanges    []StateChange    `json:"state_changes"`
	NewEntities     []NewEntity      `json:"new_entities"`
	ResourceChanges []ResourceChange `json:"resource_changes"`
	Hooks           []HookCandidate  `json:"hooks"`
	Subplots        []SubplotChange  `json:"subplots"`
	WorldFacts      []StateChange    `json:"world_facts"`
	Warnings        []string         `json:"warnings"`
	Preview         bool             `json:"preview"`
}

type DataAgent struct {
	client *llm.Client
}

func NewDataAgent(client *llm.Client) *DataAgent { return &DataAgent{client: client} }

func (a *DataAgent) ExtractPreview(ctx context.Context, chapterNumber int, content, knownContext string) (*DataExtractionPreview, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, fmt.Errorf("chapter content is required")
	}
	contentRunes := []rune(content)
	if len(contentRunes) > 16000 {
		contentRunes = contentRunes[:16000]
	}
	contextRunes := []rune(knownContext)
	if len(contextRunes) > 4000 {
		contextRunes = contextRunes[:4000]
	}

	prompt := fmt.Sprintf(`你是小说数据提取 Agent。请从章节正文中提取“候选变化”，用于人工审核。
不要推测正文没有明确支持的事实。每项必须包含原文证据和 0-1 置信度。
这只是预览，禁止输出任何 SQL、删除或写入指令。

【章节】第 %d 章
【已知上下文】
%s

【章节正文】
%s

严格输出 JSON：
{
  "state_changes": [{"entity_type":"character|organization|item","entity_name":"","field":"","old_value":"","new_value":"","evidence":"","confidence":0.0}],
  "new_entities": [{"entity_type":"character|organization|item|location","name":"","description":"","attributes":{},"evidence":"","confidence":0.0}],
  "resource_changes": [{"character_name":"","resource_type":"item|ability|status|currency","resource_name":"","previous_value":"","current_value":"","evidence":"","confidence":0.0}],
  "hooks": [{"hook_type":"foreshadowing|mystery|promise","description":"","priority":"low|medium|high","expected_resolution_chapter":null,"related_characters":[],"evidence":"","confidence":0.0}],
  "subplots": [{"name":"","change_type":"introduced|advanced|resolved","description":"","status":"active|resolved","characters":[],"evidence":"","confidence":0.0}],
  "world_facts": [{"entity_type":"world","entity_name":"","field":"","old_value":"","new_value":"","evidence":"","confidence":0.0}],
  "warnings": []
}`, chapterNumber, string(contextRunes), string(contentRunes))

	result, err := a.client.GenerateJSON(ctx, prompt, llm.WithTemperature(0.2), llm.WithMaxTokens(6000))
	if err != nil {
		return nil, fmt.Errorf("extract chapter data: %w", err)
	}
	preview, err := decodeDataExtraction(result)
	if err != nil {
		return nil, err
	}
	preview.Preview = true
	normalizeExtraction(preview)
	return preview, nil
}

func decodeDataExtraction(result map[string]interface{}) (*DataExtractionPreview, error) {
	arrayKeys := []string{"state_changes", "new_entities", "resource_changes", "hooks", "subplots", "world_facts", "warnings"}
	for _, key := range arrayKeys {
		items, ok := result[key].([]interface{})
		if !ok {
			return nil, fmt.Errorf("decode extraction response: %s must be an array", key)
		}
		if len(items) > 200 {
			return nil, fmt.Errorf("decode extraction response: %s exceeds 200 items", key)
		}
	}
	for index, warning := range result["warnings"].([]interface{}) {
		text, ok := warning.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, fmt.Errorf("decode extraction response: warnings[%d] must be a non-empty string", index)
		}
	}
	data, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("encode extraction response: %w", err)
	}
	var preview DataExtractionPreview
	if err := json.Unmarshal(data, &preview); err != nil {
		return nil, fmt.Errorf("decode extraction response: %w", err)
	}
	preview.Warnings = trimNonEmptyStrings(preview.Warnings)
	return &preview, nil
}

func trimNonEmptyStrings(values []string) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = strings.TrimSpace(value)
	}
	return result
}

func normalizeExtraction(preview *DataExtractionPreview) {
	preview.StateChanges = filterStateChanges(preview.StateChanges, map[string]bool{"character": true, "organization": true, "item": true})
	preview.WorldFacts = filterStateChanges(preview.WorldFacts, map[string]bool{"world": true})
	preview.NewEntities = filterNewEntities(preview.NewEntities)
	preview.ResourceChanges = filterResourceChanges(preview.ResourceChanges)
	preview.Hooks = filterHooks(preview.Hooks)
	preview.Subplots = filterSubplots(preview.Subplots)
}

func allowedValue(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func trimNonEmptyStringsFiltered(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func trimStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if key != "" && value != "" {
			result[key] = value
		}
	}
	return result
}

func normalizeConfidence(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func filterStateChanges(items []StateChange, allowedTypes map[string]bool) []StateChange {
	out := make([]StateChange, 0, len(items))
	for _, item := range items {
		item.EntityType = strings.ToLower(strings.TrimSpace(item.EntityType))
		item.EntityName, item.Field, item.OldValue, item.NewValue, item.Evidence = strings.TrimSpace(item.EntityName), strings.TrimSpace(item.Field), strings.TrimSpace(item.OldValue), strings.TrimSpace(item.NewValue), strings.TrimSpace(item.Evidence)
		item.Confidence = normalizeConfidence(item.Confidence)
		if allowedTypes[item.EntityType] && item.EntityName != "" && item.Field != "" && item.NewValue != "" && item.Evidence != "" {
			out = append(out, item)
		}
	}
	return out
}

func filterNewEntities(items []NewEntity) []NewEntity {
	out := make([]NewEntity, 0, len(items))
	for _, item := range items {
		item.EntityType = strings.ToLower(strings.TrimSpace(item.EntityType))
		item.Name, item.Description, item.Evidence = strings.TrimSpace(item.Name), strings.TrimSpace(item.Description), strings.TrimSpace(item.Evidence)
		item.Attributes = trimStringMap(item.Attributes)
		item.Confidence = normalizeConfidence(item.Confidence)
		if allowedValue(item.EntityType, "character", "organization", "item", "location") && item.Name != "" && item.Description != "" && item.Evidence != "" {
			out = append(out, item)
		}
	}
	return out
}

func filterResourceChanges(items []ResourceChange) []ResourceChange {
	out := make([]ResourceChange, 0, len(items))
	for _, item := range items {
		item.ResourceType = strings.ToLower(strings.TrimSpace(item.ResourceType))
		item.CharacterName, item.ResourceName, item.PreviousValue, item.CurrentValue, item.Evidence = strings.TrimSpace(item.CharacterName), strings.TrimSpace(item.ResourceName), strings.TrimSpace(item.PreviousValue), strings.TrimSpace(item.CurrentValue), strings.TrimSpace(item.Evidence)
		item.Confidence = normalizeConfidence(item.Confidence)
		if allowedValue(item.ResourceType, "item", "ability", "status", "currency") && item.CharacterName != "" && item.ResourceName != "" && item.CurrentValue != "" && item.Evidence != "" {
			out = append(out, item)
		}
	}
	return out
}

func filterHooks(items []HookCandidate) []HookCandidate {
	out := make([]HookCandidate, 0, len(items))
	for _, item := range items {
		item.HookType, item.Priority = strings.ToLower(strings.TrimSpace(item.HookType)), strings.ToLower(strings.TrimSpace(item.Priority))
		item.Description, item.Evidence = strings.TrimSpace(item.Description), strings.TrimSpace(item.Evidence)
		item.RelatedCharacters = trimNonEmptyStringsFiltered(item.RelatedCharacters)
		item.Confidence = normalizeConfidence(item.Confidence)
		validChapter := item.ExpectedResolutionChapter == nil || *item.ExpectedResolutionChapter > 0
		if allowedValue(item.HookType, "foreshadowing", "mystery", "promise") && allowedValue(item.Priority, "low", "medium", "high") && validChapter && item.Description != "" && item.Evidence != "" {
			out = append(out, item)
		}
	}
	return out
}

func filterSubplots(items []SubplotChange) []SubplotChange {
	out := make([]SubplotChange, 0, len(items))
	for _, item := range items {
		item.ChangeType, item.Status = strings.ToLower(strings.TrimSpace(item.ChangeType)), strings.ToLower(strings.TrimSpace(item.Status))
		item.Name, item.Description, item.Evidence = strings.TrimSpace(item.Name), strings.TrimSpace(item.Description), strings.TrimSpace(item.Evidence)
		item.Characters = trimNonEmptyStringsFiltered(item.Characters)
		item.Confidence = normalizeConfidence(item.Confidence)
		if allowedValue(item.ChangeType, "introduced", "advanced", "resolved") && allowedValue(item.Status, "active", "resolved") && item.Name != "" && item.Description != "" && item.Evidence != "" {
			out = append(out, item)
		}
	}
	return out
}
