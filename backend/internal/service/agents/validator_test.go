package agents

import "testing"

func TestContinuityValidatorDetectsRepeatedOpeningAndHardCut(t *testing.T) {
	validator := NewContinuityValidator()
	previous := "夜色沉沉，他抬头看向远处。" + string(make([]rune, 350))
	current := "夜色沉沉，他抬头看向远处。\n\n突然有人出现。我看见他走来。"
	issues := validator.ValidateChapter(current, previous, nil)
	if len(issues) == 0 {
		t.Fatal("expected continuity issues")
	}
	if validator.Summary(issues) == "✅ 连贯性检查通过！" {
		t.Fatal("summary must report detected issues")
	}
}

func TestContinuityValidatorPassesSimpleFirstChapter(t *testing.T) {
	validator := NewContinuityValidator()
	issues := validator.ValidateChapter("清晨的院子很安静。", "", nil)
	if len(issues) != 0 {
		t.Fatalf("unexpected issues: %#v", issues)
	}
}
