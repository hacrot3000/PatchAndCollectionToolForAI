package tasks

import (
	"os/exec"
	"path/filepath"
	"runtime"
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
		"TD_DIR=$(mktemp -d); export TD_DIR",
		"td_wait_status",
		"timeout --foreground",
		"::taskdeck-output version=",
		"grep -F",
		"TASKDECK_OUTPUT_",
		"td_node_",
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

func TestWorkflowExecutionRunsParallelDependenciesAndPropagatesOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("workflow compiler currently targets the existing Bash task runner")
	}
	if _, err := exec.LookPath("/bin/bash"); err != nil {
		t.Skip("/bin/bash unavailable")
	}
	workspace := t.TempDir()
	aStart := filepath.Join(workspace, "a.start")
	bStart := filepath.Join(workspace, "b.start")

	waitForOther := func(self, other, output string) Task {
		task := workflowTask(self, nil, map[string]any{"outputs": []any{"version"}})
		task.Command = "bash"
		task.Args = []any{"-c", "touch "+shellQuote(filepath.Join(workspace, strings.ToLower(self)+".start"))+"; for i in $(seq 1 40); do [ -f "+shellQuote(other)+" ] && { echo '::taskdeck-output version="+output+"'; exit 0; }; sleep 0.05; done; exit 9"}
		return task
	}
	a := waitForOther("A", bStart, "1.2.3")
	b := waitForOther("B", aStart, "ignored")

	deploy := workflowTask("Deploy", []any{"A", "B"}, nil)
	deploy.Command = "printf"
	deploy.Args = []any{"deploy-${output:A.version}\n"}

	spec, err := ResolveWorkflowExecution([]Task{a, b, deploy}, "Deploy", workspace, nil)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(spec.Command, spec.Args...)
	cmd.Dir = spec.Cwd
	cmd.Env = spec.Env
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("workflow failed: %v\n%s\nSCRIPT:\n%s", err, output, spec.Args[1])
	}
	text := string(output)
	if !strings.Contains(text, "deploy-1.2.3") {
		t.Fatalf("workflow output dependency was not propagated:\n%s", text)
	}
}

func TestWorkflowExecutionSequenceDoesNotPretendParallel(t *testing.T) {
	build := workflowTask("Build", nil, nil)
	testTask := workflowTask("Test", nil, nil)
	pkg := workflowTask("Package", []any{"Build", "Test"}, nil)
	pkg.Raw["dependsOrder"] = "sequence"
	spec, err := ResolveWorkflowExecution([]Task{build, testTask, pkg}, "Package", t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	script := spec.Args[1]
	buildCall := "td_node_" + workflowNodeID("Build") + " || true"
	testCall := "td_node_" + workflowNodeID("Test") + " || true"
	if !strings.Contains(script, buildCall) || !strings.Contains(script, testCall) {
		t.Fatalf("sequential dependency calls missing:\n%s", script)
	}
}
