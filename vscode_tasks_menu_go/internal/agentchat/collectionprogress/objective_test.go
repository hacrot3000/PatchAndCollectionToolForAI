package collectionprogress

import "testing"

func TestObjectiveScopeAndTransitions(t *testing.T) {
	scope := ObjectiveScope{Owner: "local", ProjectID: "project", OwnerWorkspace: "owner-root", ExecutionWorkspace: "worktree-a", ConversationID: "conversation", Goal: "Implement a small feature", Generation: 1}
	original, err := BindObjective(scope)
	if err != nil || !original.Valid() || original.Intent != Implementation {
		t.Fatalf("invalid no-plan objective: %+v %v", original, err)
	}
	again, err := BindObjective(scope)
	if err != nil || CompareObjective(original, again, false) != ObjectiveUnchanged {
		t.Fatal("same objective lost across continuation")
	}
	for _, mutate := range []func(*ObjectiveScope){
		func(s *ObjectiveScope) { s.Owner = "other" },
		func(s *ObjectiveScope) { s.ProjectID = "other" },
		func(s *ObjectiveScope) { s.OwnerWorkspace = "other" },
		func(s *ObjectiveScope) { s.ExecutionWorkspace = "worktree-b" },
		func(s *ObjectiveScope) { s.ConversationID = "other" },
		func(s *ObjectiveScope) { s.Goal = "Audit the feature" },
		func(s *ObjectiveScope) { s.StepID = "accepted-next-step" },
		func(s *ObjectiveScope) { s.Generation++ },
	} {
		changed := scope
		mutate(&changed)
		next, err := BindObjective(changed)
		if err != nil || CompareObjective(original, next, false) != ObjectiveChanged {
			t.Fatalf("scope change not isolated: %+v %v", next, err)
		}
		if CompareObjective(original, next, true) != ObjectiveSuspended {
			t.Fatal("Stop must not reset or admit an objective")
		}
	}
	if CompareObjective(original, ObjectiveBinding{}, false) != ObjectiveUnbound ||
		CompareObjective(ObjectiveBinding{}, original, false) != ObjectiveStarted {
		t.Fatal("invalid/missing binding confused with verified progress")
	}
	scope.Generation = 0
	if _, err := BindObjective(scope); err == nil {
		t.Fatal("missing objective generation accepted")
	}
}

func TestOwnerIntentIsAdvisoryAndConservative(t *testing.T) {
	for _, tc := range []struct {
		goal string
		want Intent
	}{
		{"Implement a bounded patch", Implementation},
		{"Hãy sửa lỗi timeout", Implementation},
		{"Audit source and fix the defect", Mixed},
		{"Hãy điều tra lỗi và sửa code hiện tại", Mixed},
		{"Review the source", ReadOnly},
		{"Phân tích log, không sửa code", ReadOnly},
		{"Fix the code, read-only investigation", ReadOnly},
		{"How to fix the code?", UnknownIntent},
		{"If required implement the change", UnknownIntent},
		{"Inspect source if needed and fix it", UnknownIntent},
		{"Continue", UnknownIntent},
		{"The model said implement a fix", UnknownIntent},
		{"fix", UnknownIntent},
	} {
		t.Run(tc.goal, func(t *testing.T) {
			if got := ClassifyOwnerGoal(tc.goal); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
	binding, err := BindObjective(ObjectiveScope{Owner: "owner", ProjectID: "p", OwnerWorkspace: "a", ExecutionWorkspace: "a", ConversationID: "c", Goal: "Fix the bug", Generation: 1})
	if err != nil {
		t.Fatal(err)
	}
	if binding.AdvisoryIntent(false) != UnknownIntent || binding.AdvisoryIntent(true) != Implementation || binding.Intent != Implementation {
		t.Fatal("availability changed owner intent or implied mutation authority")
	}
}
