package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestCommandPresetRunnerFoundation(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/commandpresets.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"view.meta.task_id!==0",
		"view.meta.status!=='running'",
		"runs.has(view.meta.id)",
		"for(let index=0;index<preset.commands.length;index++)",
		"const rc=await executeCommand(view,run,preset.commands[index],index)",
		"if(rc!==0)",
		"view.ws.send(commandPayload(view,command,run.token,index))",
		"term", // module is xterm/session oriented
		"taskmenu:output",
		"taskmenu:session",
		"markerPattern(run.token,index)",
		"\x1eVTM_PRESET:",
		"\x1f",
		"Preset commands support bash/sh/zsh/dash/ksh/ash/fish terminals",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("commandpresets.js missing %q", want)
		}
	}
	if strings.Contains(js, "/api/broadcast") {
		t.Fatal("preset runner must not fan commands through Broadcast All/Group")
	}
	if !strings.Contains(js, "printf \'\\036VTM_PRESET:") {
		t.Fatal("preset runner must ask shell printf to emit RS sentinel with one backslash escape")
	}
	if strings.Contains(js, "printf \'\\\\036VTM_PRESET:") {
		t.Fatal("preset runner sentinel escape is double-escaped and would print literal text")
	}
}
