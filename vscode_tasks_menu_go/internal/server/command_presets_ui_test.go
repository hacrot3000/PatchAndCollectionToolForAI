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
		"const payload=commandPayload(view,command,run.token,index)",
		"view.ws.send(payload)",
		"term",
		"taskmenu:output",
		"taskmenu:session",
		"markerPattern(run.token,index)",
		"\\x1eVTM_PRESET:",
		"\\x1f",
		"Preset commands support bash/sh/zsh/dash/ksh/ash/fish terminals",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("commandpresets.js missing %q", want)
		}
	}
	if strings.Contains(js, "/api/broadcast") {
		t.Fatal("preset runner must not fan commands through Broadcast All/Group")
	}
	if !strings.Contains(js, `printf \'\\036VTM_PRESET:`) {
		t.Fatal("preset runner must ask shell printf to emit RS sentinel with one backslash escape")
	}
	if strings.Contains(js, `printf \'\\\\036VTM_PRESET:`) {
		t.Fatal("preset runner sentinel escape is double-escaped and would print literal text")
	}
}

func TestCommandPresetManagerAndContextActions(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/commandpresets.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"Preset command (running)",
		"Preset command",
		"if(!view||view.meta.task_id!==0)return []",
		"preset.name+' — '+(firstCommandSnippet(preset)",
		"Manage presets…",
		"Command presets",
		"＋ Add preset",
		"Preset name",
		"Commands — executed sequentially; stop on non-zero exit code",
		"＋ Add command",
		"Delete preset",
		"Close",
		"action:'create'",
		"action:'update'",
		"action:'delete'",
		"Discard unsaved preset changes?",
		"textarea",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("commandpresets.js missing manager/context behavior %q", want)
		}
	}
}

func TestCommandPresetLoaderOrder(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	next := string(data)
	broadcast := strings.Index(next, "import '/featuremods/broadcast.js';")
	presets := strings.Index(next, "import '/featuremods/commandpresets.js';")
	context := strings.Index(next, "import '/featuremods/tabcontext.js';")
	if broadcast < 0 || presets < 0 || context < 0 {
		t.Fatalf("missing loader entries: broadcast=%d presets=%d context=%d", broadcast, presets, context)
	}
	if !(broadcast < presets && presets < context) {
		t.Fatalf("command presets must load after broadcast and before tab context: broadcast=%d presets=%d context=%d", broadcast, presets, context)
	}
}

func TestTabContextSupportsPresetSubmenus(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/tabcontext.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"TaskMenuCommandPresets?.contextActions?.(view)",
		"addCustomActions('',presetActions)",
		"Array.isArray(action.children)&&action.children.length",
		"openCustomSubmenu(item,action.children)",
		"tab-context-submenu",
		"positionSubmenu(owner)",
		"ownerRect.left-rect.width-4",
		"submenu?.contains(target)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("tabcontext.js missing preset submenu support %q", want)
		}
	}
}

func TestCommandPresetDisplayHidesInternalWrapper(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/commandpresets.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"app.addOutputFilter(filterPresetDisplay)",
		"view.term.write(displayCommandText(command))",
		"payloadEchoLineCount(payload)",
		"run.echoLinesRemaining",
		"stripPresetSentinel(run,text)",
		"return out||null",
		"run.displayActive=typeof app.addOutputFilter==='function'",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("commandpresets.js missing display filtering behavior %q", want)
		}
	}
	if strings.Contains(js, "stty -echo") {
		t.Fatal("preset display cleanup must not disable terminal echo with stty")
	}
}

func TestCoreOutputFilterKeepsRawOutputEvent(t *testing.T) {
	if !strings.Contains(appJS, "const outputFilters=[]") {
		t.Fatal("core UI missing output filter registry")
	}
	if !strings.Contains(appJS, "addOutputFilter") || !strings.Contains(appJS, "filterOutput(view,data)") {
		t.Fatal("core UI missing output filter API")
	}
	write := strings.Index(appJS, "writeOutput(view,filterOutput(view,data));")
	rawEvent := strings.Index(appJS, "taskmenu:output")
	if write < 0 || rawEvent < 0 || write > rawEvent {
		t.Fatal("core must filter display output while still dispatching raw taskmenu:output afterwards")
	}
}
