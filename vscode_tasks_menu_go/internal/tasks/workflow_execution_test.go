package tasks

import (
	"strings"
	"testing"
)

func TestResolveWorkflowExecutionCompilesRetryTimeoutConditionAndOutputs(t *testing.T) {
	build := workflowTask("Build", nil, map[string]any{
		"retry": 2.0,
		"timeoutSeconds": 30.0,
		"outputs": []any{"version"},
	})
	build.Command = "printf"
	build.Args = []any{"::taskdeck-output version=1.2.3\n"}

	deploy := workflowTask("Deploy", "Build", map[string]any{"condition": "success"})
	deploy.Command = "echo"
	deploy.Args = []any{"deploy-${output:Build.version}"}

	spec, err := ResolveWorkflowExecution([]Task{build, deploy}, "Deploy", t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Command != "/bin/bash" || len(spec.Args) != 2 || spec.Args[0] != "-c" {
		t.Fatalf("workflow execution=%+v", spec)
	}
	script := spec.Args[1]
	for _, want := range []string{
		"declare -A TD_STATUS",
		"[TaskDeck workflow] $name",
		"timeout --foreground",
		"::taskdeck-output version=",
		"grep -F",
		"TASKDECK_OUTPUT_",
		"Deploy",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("compiled workflow missing %q\n%s", want, script)
		}
	}
	if strings.Contains(script, "${output:Build.version}") {
		t.Fatalf("unresolved output reference remains in workflow script:\n%s", script)
	}
}

func TestResolveWorkflowExecutionRejectsUndeclaredOutputDependency(t *testing.T) {
	build := workflowTask("Build", nil, nil)
	deploy := workflowTask("Deploy", "Build", nil)
	deploy.Command = "echo"
	deploy.Args = []any{"${output:Build.version}"}
	if _, err := ResolveWorkflowExecution([]Task{build, deploy}, "Deploy", t.TempDir(), nil); err == nil {
		t.Fatal("undeclared workflow output dependency was accepted")
	}
}

func TestResolveWorkflowExecutionKeepsSingleTaskRunner(t *testing.T) {
	task := workflowTask("One", nil, nil)
	task.Command = "echo"
	task.Args = []any{"hello"}
	spec, err := ResolveWorkflowExecution([]Task{task}, "One", t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Label != "One" || spec.Command != "/bin/bash" {
		t.Fatalf("single task did not retain normal runner semantics: %+v", spec)
	}
	if strings.Contains(spec.Preview, "Task workflow") {
		t.Fatalf("single task unexpectedly became workflow: %+v", spec)
	}
}
