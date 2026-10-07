package agents

import "testing"

func validAuditMap() map[string]interface{} {
	return map[string]interface{}{
		"overall_score": 8.5,
		"summary":       " 总体良好 ",
		"dimensions": []interface{}{map[string]interface{}{
			"name": " 情节 ", "score": 8.0, "feedback": " 连贯 ",
		}},
		"issues": []interface{}{map[string]interface{}{
			"type": "风格", "severity": "WARNING", "description": "重复", "location": "第3段",
		}},
		"suggestions": []interface{}{" 精简重复表达 "},
	}
}

func TestParseAuditResultValidatesAndNormalizesSchema(t *testing.T) {
	result, err := parseAuditResult(validAuditMap())
	if err != nil {
		t.Fatal(err)
	}
	if result.OverallScore != 8.5 || result.Summary != "总体良好" || result.Dimensions[0].Name != "情节" || result.Issues[0].Severity != "warning" || result.Suggestions[0] != "精简重复表达" {
		t.Fatalf("unexpected audit: %#v", result)
	}
}

func TestParseAuditResultRejectsInvalidSchema(t *testing.T) {
	tests := []func(map[string]interface{}){
		func(value map[string]interface{}) { delete(value, "overall_score") },
		func(value map[string]interface{}) { value["overall_score"] = 11.0 },
		func(value map[string]interface{}) { value["summary"] = " " },
		func(value map[string]interface{}) { value["dimensions"] = []interface{}{} },
		func(value map[string]interface{}) {
			value["dimensions"] = []interface{}{map[string]interface{}{"name": "情节", "score": -1.0, "feedback": "差"}}
		},
		func(value map[string]interface{}) { value["issues"] = "none" },
		func(value map[string]interface{}) {
			value["issues"] = []interface{}{map[string]interface{}{"type": "风格", "severity": "unknown", "description": "问题", "location": "段落"}}
		},
		func(value map[string]interface{}) { value["suggestions"] = []interface{}{} },
	}
	for index, mutate := range tests {
		value := validAuditMap()
		mutate(value)
		if _, err := parseAuditResult(value); err == nil {
			t.Fatalf("invalid audit schema %d was accepted: %#v", index, value)
		}
	}
}
