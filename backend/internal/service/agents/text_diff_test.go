package agents

import (
	"strings"
	"testing"
)

func TestTextDifferFindsUnicodeChange(t *testing.T) {
	result := NewTextDiffer().Preview("开头。旧内容。结尾", "开头。新内容更长。结尾", 100, 3)
	if !result.Changed || result.Original != "旧内容" || result.Candidate != "新内容更长" {
		t.Fatalf("unexpected diff: %#v", result)
	}
	if result.RemovedRunes != 3 || result.AddedRunes != 5 || !result.Preview {
		t.Fatalf("unexpected counts: %#v", result)
	}
}

func TestTextDifferBoundsChangedRegion(t *testing.T) {
	result := NewTextDiffer().Preview("前"+strings.Repeat("旧", 1000)+"后", "前"+strings.Repeat("新", 1000)+"后", 100, 10)
	if !result.Truncated || len([]rune(result.Original)) != 100 || len([]rune(result.Candidate)) != 100 {
		t.Fatalf("unexpected bounded diff: %#v", result)
	}
}

func TestTextDifferHandlesEqualText(t *testing.T) {
	result := NewTextDiffer().Preview("相同", "相同", 100, 10)
	if result.Changed || result.AddedRunes != 0 || result.RemovedRunes != 0 {
		t.Fatalf("unexpected equal diff: %#v", result)
	}
}

func TestTextDifferAllowsEmptyCandidate(t *testing.T) {
	result := NewTextDiffer().Preview("删除全文🙂", "", 100, 10)
	if !result.Changed || result.Candidate != "" || result.RemovedRunes != 5 || result.AddedRunes != 0 {
		t.Fatalf("unexpected full deletion diff: %#v", result)
	}
}
