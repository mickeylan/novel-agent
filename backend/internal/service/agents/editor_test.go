package agents

import "testing"

func TestValidateEditorResultRejectsWhitespace(t *testing.T) {
	for _, result := range []string{"", "   ", "\n\t"} {
		if _, err := validateEditorResult(result); err == nil {
			t.Fatalf("whitespace editor result was accepted: %q", result)
		}
	}
}

func TestValidateEditorResultTrimsValidContent(t *testing.T) {
	result, err := validateEditorResult("  有效润色结果\n")
	if err != nil {
		t.Fatal(err)
	}
	if result != "有效润色结果" {
		t.Fatalf("unexpected normalized editor result: %q", result)
	}
}
