package tasks

import "testing"

func workflowTask(label string, depends any, workflow map[string]any) Task {
	raw := map[string]any{"label": label, "type": "shell", "command": "echo " + label}
	if depends != nil {
		raw["dependsOn"] = depends
	}
	if workflow != nil {
		raw["taskdeck"] = map[string]any{"workflow": workflow}
	}
	return Task{ID: len(label)+1, Label: label, Type: "shell", Command: "echo " + label, Raw: raw}
}

func TestBuildWorkflowGraphTopologicalOrderAndPolicies(t *testing.T) {
	items := []Task{
		workflowTask("Build", nil, map[string]any{"retry": 2.0, "timeoutSeconds": 45.0, "outputs": []any{"artifact", "version"}}),
		workflowTask("Test", "Build", map[string]any{"continueOnError": true}),
		workflowTask("Package", []any{"Build", "Test"}, map[string]any{"condition": "always"}),
	}
	items[2].Raw["dependsOrder"] = "sequence"
	graph, err := BuildWorkflowGraph(items, "Package")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Build", "Test", "Package"}
	if len(graph.Order) != len(want) {
		t.Fatalf("order=%v", graph.Order)
	}
	for i := range want {
		if graph.Order[i] != want[i] {
			t.Fatalf("order=%v want=%v", graph.Order, want)
		}
	}
	if len(graph.Edges) != 3 {
		t.Fatalf("edges=%v", graph.Edges)
	}
	var build, test, pkg WorkflowNode
	for _, node := range graph.Nodes {
		switch node.Label {
		case "Build":
			build = node
		case "Test":
			test = node
		case "Package":
			pkg = node
		}
	}
	if build.Policy.Retry != 2 || build.Policy.TimeoutSeconds != 45 || len(build.Policy.Outputs) != 2 {
		t.Fatalf("build policy=%+v", build.Policy)
	}
	if !test.Policy.ContinueOnError {
		t.Fatalf("test policy=%+v", test.Policy)
	}
	if pkg.Policy.Condition != "always" || pkg.DependsOrder != "sequence" {
		t.Fatalf("package node=%+v", pkg)
	}
}

func TestBuildWorkflowGraphRejectsMissingAndCycles(t *testing.T) {
	_, err := BuildWorkflowGraph([]Task{workflowTask("Deploy", "Package", nil)}, "Deploy")
	if err == nil {
		t.Fatal("missing dependency was accepted")
	}

	a := workflowTask("A", "B", nil)
	b := workflowTask("B", "A", nil)
	if _, err := BuildWorkflowGraph([]Task{a, b}, "A"); err == nil {
		t.Fatal("dependency cycle was accepted")
	}
}

func TestWorkflowPolicyBoundsInvalidValues(t *testing.T) {
	task := workflowTask("A", nil, map[string]any{
		"retry": 1000.0,
		"timeoutSeconds": -1.0,
		"condition": "shell-expression",
	})
	policy := WorkflowPolicyForTask(task)
	if policy.Retry != 0 || policy.TimeoutSeconds != 0 || policy.Condition != "" {
		t.Fatalf("unsafe workflow values were accepted: %+v", policy)
	}
}

func TestWorkflowInputsIncludeDependenciesInTopologicalOrder(t *testing.T) {
	build := workflowTask("Build", nil, nil)
	build.Inputs = []Input{{ID:"target", Type:"pickString", Description:"Target", Default:"debug"}}
	deploy := workflowTask("Deploy", "Build", nil)
	deploy.Inputs = []Input{{ID:"host", Type:"promptString", Description:"Host"}}
	inputs, err := WorkflowInputs([]Task{build, deploy}, "Deploy")
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) != 2 || inputs[0].ID != "target" || inputs[1].ID != "host" {
		t.Fatalf("workflow inputs=%+v", inputs)
	}
}


func TestBuildWorkflowGraphRejectsInvalidWorkflowConfiguration(t *testing.T) {
	cases := []struct{
		name string
		task Task
	}{
		{"fractional retry", workflowTask("A", nil, map[string]any{"retry":1.5})},
		{"retry too high", workflowTask("A", nil, map[string]any{"retry":11.0})},
		{"timeout too high", workflowTask("A", nil, map[string]any{"timeoutSeconds":86401.0})},
		{"invalid condition", workflowTask("A", nil, map[string]any{"condition":"sometimes"})},
		{"invalid output", workflowTask("A", nil, map[string]any{"outputs":[]any{"bad output"}})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := BuildWorkflowGraph([]Task{tc.task}, tc.task.Label); err == nil {
				t.Fatalf("invalid workflow configuration was accepted: %+v", tc.task.Raw)
			}
		})
	}
}
