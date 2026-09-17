package tasks

import (
	"strings"
	"testing"
)

func TestLoadAttachesReferencedPromptAndPickInputs(t *testing.T) {
	workspace := t.TempDir()
	writeTasksFile(t, workspace, `{
		"version":"2.0.0",
		"inputs":[
			{"id":"version","type":"promptString","description":"Version","default":"1.2.3"},
			{"id":"target","type":"pickString","description":"Target","options":["main","gateway"],"default":"main"},
			{"id":"unused","type":"promptString","description":"Unused"}
		],
		"tasks":[{"label":"Deploy","type":"shell","command":"echo ${input:version}","args":["${input:target}"]}]
	}`)
	items, err := Load(workspace)
	if err != nil { t.Fatal(err) }
	if len(items) != 1 { t.Fatalf("tasks=%d want 1", len(items)) }
	inputs := items[0].Inputs
	if len(inputs) != 2 { t.Fatalf("inputs=%#v want 2 referenced inputs", inputs) }
	if inputs[0].ID != "version" || inputs[0].Type != "promptString" || inputs[0].Default != "1.2.3" { t.Fatalf("prompt input=%#v", inputs[0]) }
	if inputs[1].ID != "target" || inputs[1].Type != "pickString" || len(inputs[1].Options) != 2 || inputs[1].Options[1] != "gateway" { t.Fatalf("pick input=%#v", inputs[1]) }
}

func TestLoadMarksMissingReferencedInput(t *testing.T) {
	workspace := t.TempDir()
	writeTasksFile(t, workspace, `{"tasks":[{"label":"A","type":"shell","command":"echo ${input:notDefined}"}]}`)
	items, err := Load(workspace)
	if err != nil { t.Fatal(err) }
	if len(items[0].Inputs) != 1 || items[0].Inputs[0].Type != "missing" { t.Fatalf("inputs=%#v", items[0].Inputs) }
}

func TestResolveExecutionWithInputsSubstitutesCommandArgsCwdAndEnv(t *testing.T) {
	workspace := t.TempDir()
	task := Task{ID:42, Label:"Parameterized", Type:"shell", Command:"printf", Args:[]any{"%s %s", "${input:name}", "${input:target}"}, Options:map[string]any{"cwd":"${workspaceFolder}/${input:subdir}", "env":map[string]any{"MODE":"${input:target}"}}}
	spec, err := ResolveExecutionWithInputs(task, workspace, map[string]string{"name":"hello world","target":"gateway","subdir":"tmp"})
	if err != nil { t.Fatal(err) }
	if !strings.Contains(spec.Preview, "'hello world'") || !strings.Contains(spec.Preview, "gateway") { t.Fatalf("preview=%q", spec.Preview) }
	if !strings.HasSuffix(spec.Cwd, "/tmp") { t.Fatalf("cwd=%q", spec.Cwd) }
	found := false;for _, item := range spec.Env { if item == "MODE=gateway" { found=true } }
	if !found { t.Fatalf("MODE override missing from env") }
}

func TestResolveExecutionWithInputsRejectsMissingValue(t *testing.T) {
	_, err := ResolveExecutionWithInputs(Task{Label:"A", Type:"shell", Command:"echo ${input:required}"}, t.TempDir(), nil)
	if err == nil || !strings.Contains(err.Error(), `thiếu giá trị cho input "required"`) { t.Fatalf("err=%v", err) }
}
