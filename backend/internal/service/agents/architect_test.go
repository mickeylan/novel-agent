package agents

import (
	"math"
	"testing"
)

func validThreeActMap() map[string]interface{} {
	return map[string]interface{}{
		"act1_setup": " 设定 ", "act1_inciting_incident": "事件", "act1_plot_point1": "转折一",
		"act2_confrontation": "对抗", "act2_midpoint": "中点", "act2_plot_point2": "转折二",
		"act3_resolution": "解决", "act3_climax": "高潮", "act3_denouement": "收束",
	}
}

func validChapterOutlineMap() map[string]interface{} {
	return map[string]interface{}{
		"title": " 标题 ", "summary": "概要", "setting": "场景", "conflict": "冲突",
		"opening_type": "悬念", "cliffhanger_type": "危机", "chapter_type": "推进章", "hook_plan": "伏笔",
		"key_events": []interface{}{" 事件一 "}, "beat_outline": []interface{}{" 节拍一 "}, "word_count_target": 5000.0,
	}
}

func TestDecodeThreeActStructureRequiresAllFields(t *testing.T) {
	result, err := decodeThreeActStructure(validThreeActMap())
	if err != nil || result.Act1Setup != "设定" {
		t.Fatalf("unexpected structure: %#v, %v", result, err)
	}
	invalid := validThreeActMap()
	invalid["act3_climax"] = " "
	if _, err := decodeThreeActStructure(invalid); err == nil {
		t.Fatal("expected missing act field error")
	}
}

func TestDecodeChapterOutlineValidatesSchema(t *testing.T) {
	result, err := decodeChapterOutline(validChapterOutlineMap(), 3)
	if err != nil || result.ChapterNumber != 3 || result.Title != "标题" || result.KeyEvents[0] != "事件一" || result.WordCountTarget != 5000 {
		t.Fatalf("unexpected outline: %#v, %v", result, err)
	}
	tests := []func(map[string]interface{}){
		func(value map[string]interface{}) { value["title"] = "" },
		func(value map[string]interface{}) { value["key_events"] = []interface{}{} },
		func(value map[string]interface{}) { value["beat_outline"] = []interface{}{1} },
		func(value map[string]interface{}) { value["word_count_target"] = 99.0 },
		func(value map[string]interface{}) { value["word_count_target"] = 5000.5 },
		func(value map[string]interface{}) { value["word_count_target"] = math.NaN() },
		func(value map[string]interface{}) { value["word_count_target"] = math.Inf(1) },
	}
	for index, mutate := range tests {
		value := validChapterOutlineMap()
		mutate(value)
		if _, err := decodeChapterOutline(value, 1); err == nil {
			t.Fatalf("invalid outline schema %d accepted: %#v", index, value)
		}
	}
}

func TestGenerateChapterOutlineRejectsInvalidPositionBeforeLLM(t *testing.T) {
	architect := &Architect{}
	if _, err := architect.GenerateChapterOutline(t.Context(), nil, 1, 10, "前提"); err == nil {
		t.Fatal("expected nil structure error")
	}
	if _, err := architect.GenerateChapterOutline(t.Context(), &ThreeActStructure{}, 11, 10, "前提"); err == nil {
		t.Fatal("expected chapter range error")
	}
}
