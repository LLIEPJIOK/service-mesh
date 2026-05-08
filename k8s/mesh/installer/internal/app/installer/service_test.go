package installer

import "testing"

func TestBuildPlanOrder(t *testing.T) {
	plan := BuildPlan("mesh-system")
	if len(plan) != 7 {
		t.Fatalf("unexpected plan size: %d", len(plan))
	}
	if plan[0] == plan[6] {
		t.Fatalf("plan order looks invalid")
	}
}
