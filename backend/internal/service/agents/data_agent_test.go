package agents

import "testing"

func TestNormalizeExtractionDropsUnsupportedCandidatesAndClampsConfidence(t *testing.T) {
	preview := &DataExtractionPreview{
		StateChanges: []StateChange{
			{EntityName: " 林舟 ", Field: " 状态 ", NewValue: " 受伤 ", Evidence: " 手臂流血 ", Confidence: 1.5},
			{EntityName: "无证据", Field: "状态", NewValue: "未知"},
		},
		NewEntities: []NewEntity{{Name: " 苏月 ", Evidence: " 苏月推门而入 ", Confidence: -1}},
		Hooks:       []HookCandidate{{Description: "神秘玉佩发光", Evidence: "玉佩忽然亮起", Confidence: 0.8}},
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
