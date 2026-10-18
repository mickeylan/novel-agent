package agents

import (
	"math"
	"testing"
)

func TestNormalizeConfidenceRejectsNonFiniteValues(t *testing.T) {
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if normalized := normalizeConfidence(value); normalized != 0 || math.IsNaN(normalized) || math.IsInf(normalized, 0) {
			t.Fatalf("non-finite confidence was not safely normalized: %v", normalized)
		}
	}
}

func TestNormalizeExtractionDropsUnsupportedCandidatesAndClampsConfidence(t *testing.T) {
	preview := &DataExtractionPreview{
		StateChanges: []StateChange{
			{EntityType: " CHARACTER ", EntityName: " 林舟 ", Field: " 状态 ", NewValue: " 受伤 ", Evidence: " 手臂流血 ", Confidence: 1.5},
			{EntityName: "无证据", Field: "状态", NewValue: "未知"},
		},
		NewEntities: []NewEntity{{EntityType: "character", Name: " 苏月 ", Description: " 新角色 ", Evidence: " 苏月推门而入 ", Confidence: -1}},
		Hooks:       []HookCandidate{{HookType: "mystery", Priority: "high", Description: "神秘玉佩发光", Evidence: "玉佩忽然亮起", Confidence: 0.8}},
	}
	normalizeExtraction(preview)
	if len(preview.StateChanges) != 1 || preview.StateChanges[0].Confidence != 1 {
		t.Fatalf("unexpected state changes: %#v", preview.StateChanges)
	}
	if len(preview.NewEntities) != 1 || preview.NewEntities[0].Confidence != 0 {
		t.Fatalf("unexpected new entities: %#v", preview.NewEntities)
	}
	if len(preview.Hooks) != 1 {
		t.Fatalf("expected one hook")
	}
}

func TestNormalizeExtractionFiltersInvalidEnumsAndTrimsNestedValues(t *testing.T) {
	chapter := -1
	preview := &DataExtractionPreview{
		StateChanges: []StateChange{{EntityType: "unknown", EntityName: "角色", Field: "状态", NewValue: "变化", Evidence: "证据"}},
		NewEntities:  []NewEntity{{EntityType: " location ", Name: " 地点 ", Description: " 描述 ", Attributes: map[string]string{" 颜色 ": " 红色 ", "空": " "}, Evidence: " 证据 "}},
		Hooks: []HookCandidate{
			{HookType: " mystery ", Priority: " HIGH ", Description: " 谜团 ", RelatedCharacters: []string{" 林舟 ", " "}, Evidence: " 证据 "},
			{HookType: "mystery", Priority: "high", Description: "错误章节", ExpectedResolutionChapter: &chapter, Evidence: "证据"},
		},
	}
	normalizeExtraction(preview)
	if len(preview.StateChanges) != 0 || len(preview.NewEntities) != 1 || len(preview.Hooks) != 1 {
		t.Fatalf("unexpected filtered preview: %#v", preview)
	}
	if preview.NewEntities[0].Attributes["颜色"] != "红色" || len(preview.NewEntities[0].Attributes) != 1 || len(preview.Hooks[0].RelatedCharacters) != 1 || preview.Hooks[0].Priority != "high" {
		t.Fatalf("nested values were not normalized: %#v", preview)
	}
}

func TestDecodeDataExtraction(t *testing.T) {
	result := map[string]interface{}{
		"warnings": []interface{}{" 需要人工确认 "},
		"state_changes": []interface{}{map[string]interface{}{
			"entity_type": "character", "entity_name": "林舟", "field": "status", "new_value": "受伤", "evidence": "手臂流血", "confidence": 0.9,
		}},
		"new_entities": []interface{}{}, "resource_changes": []interface{}{}, "hooks": []interface{}{},
		"subplots": []interface{}{}, "world_facts": []interface{}{},
	}
	preview, err := decodeDataExtraction(result)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.StateChanges) != 1 || len(preview.Warnings) != 1 || preview.Warnings[0] != "需要人工确认" {
		t.Fatalf("unexpected preview: %#v", preview)
	}
}

func TestDecodeDataExtractionRequiresExplicitBoundedArrays(t *testing.T) {
	base := map[string]interface{}{
		"state_changes": []interface{}{}, "new_entities": []interface{}{}, "resource_changes": []interface{}{},
		"hooks": []interface{}{}, "subplots": []interface{}{}, "world_facts": []interface{}{}, "warnings": []interface{}{},
	}
	missing := cloneExtractionMap(base)
	delete(missing, "hooks")
	if _, err := decodeDataExtraction(missing); err == nil {
		t.Fatal("expected missing hooks error")
	}
	wrongType := cloneExtractionMap(base)
	wrongType["world_facts"] = "none"
	if _, err := decodeDataExtraction(wrongType); err == nil {
		t.Fatal("expected world_facts type error")
	}
	invalidWarning := cloneExtractionMap(base)
	invalidWarning["warnings"] = []interface{}{1}
	if _, err := decodeDataExtraction(invalidWarning); err == nil {
		t.Fatal("expected warning element error")
	}
	tooMany := cloneExtractionMap(base)
	tooMany["subplots"] = make([]interface{}, 201)
	if _, err := decodeDataExtraction(tooMany); err == nil {
		t.Fatal("expected candidate count error")
	}
}

func cloneExtractionMap(source map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
