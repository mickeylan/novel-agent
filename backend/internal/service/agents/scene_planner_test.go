package agents

import (
	"strings"
	"testing"
)

func TestScenePlannerUsesEventsAndAllocatesExactWords(t *testing.T) {
	result := NewScenePlanner().Plan("忽略的大纲", []string{"相遇", "", "冲突", "相遇", "抉择"}, 1001, 8)
	if result.SceneCount != 3 || result.Source != "key_events" || result.Truncated {
		t.Fatalf("unexpected plan: %#v", result)
	}
	total := 0
	for _, scene := range result.Scenes {
		total += scene.TargetWordCount
	}
	if total != 1001 || result.Scenes[0].TargetWordCount != 334 {
		t.Fatalf("unexpected allocation: %#v", result.Scenes)
	}
}

func TestScenePlannerSplitsOutlineAndBoundsScenes(t *testing.T) {
	result := NewScenePlanner().Plan("开门。发现线索；追踪目标。发生冲突。做出选择。", nil, 500, 3)
	if result.SceneCount != 3 || result.Source != "outline" || !result.Truncated {
		t.Fatalf("unexpected plan: %#v", result)
	}
	if result.Scenes[2].ExitHook == "" || !result.Preview {
		t.Fatalf("missing hooks or preview: %#v", result)
	}
}

func TestScenePlannerFormatsPromptConstraint(t *testing.T) {
	planner := NewScenePlanner()
	guide := planner.FormatForPrompt(planner.Plan("", []string{"发现线索", "面对冲突"}, 1000, 8))
	for _, expected := range []string{"【场景执行计划】", "发现线索", "约500字", "不要输出场景编号"} {
		if !strings.Contains(guide, expected) {
			t.Fatalf("guide %q missing %q", guide, expected)
		}
	}
	if planner.FormatForPrompt(ScenePlanPreview{}) != "" {
		t.Fatal("empty plan must not produce prompt text")
	}
}

func TestScenePlannerValidatesCoverageAndOrder(t *testing.T) {
	planner := NewScenePlanner()
	plan := planner.Plan("", []string{"发现线索", "追踪目标", "面对冲突"}, 300, 8)
	result := planner.ValidateCoverage("他先面对冲突，随后终于发现线索。", plan)
	if result.CoveredScenes != 2 || result.CoverageRate != 0.667 || result.OrderValid {
		t.Fatalf("unexpected coverage: %#v", result)
	}
	if len(result.OrderViolations) != 1 || result.OrderViolations[0] != 3 {
		t.Fatalf("unexpected order violations: %#v", result.OrderViolations)
	}
	if result.Scenes[1].Covered || result.Scenes[1].FirstPosition != -1 {
		t.Fatalf("missing scene must remain uncovered: %#v", result.Scenes[1])
	}
}

func TestScenePlannerCoverageFlagsLargeLengthDeviation(t *testing.T) {
	planner := NewScenePlanner()
	plan := planner.Plan("", []string{"发现线索", "面对冲突"}, 1000, 8)
	result := planner.ValidateCoverage("发现线索。面对冲突。", plan)
	if !result.Scenes[0].LengthWarning || !result.Scenes[1].LengthWarning {
		t.Fatalf("expected length warnings: %#v", result.Scenes)
	}
}

func TestScenePlannerProvidesFallback(t *testing.T) {
	result := NewScenePlanner().Plan("", nil, 500, 8)
	if result.SceneCount != 1 || result.Source != "fallback" || result.Scenes[0].TargetWordCount != 500 {
		t.Fatalf("unexpected fallback: %#v", result)
	}
}
