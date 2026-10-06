package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestTasksJSONVisualEditorFeature(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/taskseditor.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"const PRIMARY_TASKS_PATH='.vscode/tasks.json'",
		"button.id='tasks-json-editor'",
		"button.textContent='Edit tasks.json…'",
		"Visual",
		"Raw JSON",
		"function parseTasksDocument(text)",
		"function applyTemplate(task,id)",
		"Bash script",
		"Python script",
		"Node.js script",
		"Go · test all",
		"Rust · cargo run",
		"CMake · build",
		"Gradle · build",
		"Maven · test",
		"Docker Compose · up",
		"function pickWorkspacePath(title)",
		"browser?.pickFile",
		"Choose command / executable",
		"+ File argument…",
		"MULTI-FILE / SCRIPT EXECUTION",
		"+ Add execution file",
		"executionFiles",
		"commands.join(meta.executionMode==='continue'?'; ':' && ')",
		"/api/project/file?path=",
		"expected_sha256:fileMeta.sha256",
		"method:'PUT'",
		"await app.loadTasks()",
		"event.ctrlKey||event.metaKey",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("taskseditor.js missing visual tasks editor behavior %q", want)
		}
	}
}

func TestTasksEditorLoadsBeforeGroupedSettingsMenu(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	editor := strings.Index(js, "import '/featuremods/taskseditor.js';")
	menus := strings.Index(js, "import '/featuremods/menus.js';")
	if editor < 0 || menus < 0 || editor > menus {
		t.Fatal("taskseditor.js must load before menus.js so Settings can adopt the editor button")
	}
}

func TestTasksEditorUsesExistingProjectFileMutationAPI(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/taskseditor.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	if strings.Contains(js, "/api/tasks/config") || strings.Contains(js, "/api/tasks/editor") {
		t.Fatal("tasks editor should reuse the hardened project-file read/write API instead of adding a second tasks mutation path")
	}
	for _, want := range []string{
		"app.hasPermission?.('files.read')",
		"app.hasPermission?.('files.write')",
		"expected_sha256:fileMeta.sha256",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("tasks editor missing project-file safety behavior %q", want)
		}
	}
}


func TestTasksEditorGroupsTasksAndEditsMenuGroup(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/taskseditor.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"function taskMenuGroupParts(task)",
		"function buildTaskEditorTree()",
		"if(group.length===1&&group[0]==='build')",
		"function appendTaskEditorTree(node,host)",
		"tasks-editor-group-title",
		"tasks-editor-group-children",
		"function menuGroupOptions()",
		"function makeMenuGroupInput(task)",
		"document.createElement('datalist')",
		"input.setAttribute('list',dataList.id)",
		"input.placeholder='Group/Subgroup'",
		"addField(form,'Menu group',makeMenuGroupInput(task))",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("taskseditor.js missing grouped task/menu-group behavior %q", want)
		}
	}
}

func TestTasksEditorRawSelectionJumpsWithoutRerendering(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/taskseditor.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"function selectTask(index)",
		"if(mode==='raw'&&rawArea)",
		"updateList();",
		"focusRawTask(index);",
		"function findTasksArrayStart(text)",
		"function findRawTaskRange(text,targetIndex)",
		"function focusRawTask(index)",
		"rawArea.setSelectionRange(range.start,range.end)",
		"requestAnimationFrame(()=>focusRawTask(selectedIndex))",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("taskseditor.js missing raw task navigation behavior %q", want)
		}
	}
}

func TestTasksEditorSupportsWorkspaceRoots(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/taskseditor.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"let workspaceRoots=[],selectedRootID='primary'",
		"function tasksRootBase(root)",
		"function tasksPathForRoot(root)",
		"async function loadWorkspaceRoots()",
		"app.jsonFetch('/api/workspace-roots')",
		"async function currentTasksFileExists()",
		"async function ensureCurrentTasksFile()",
		"action:'mkdir',path:currentTasksDir()",
		"action:'create_file',path:currentTasksPath()",
		"rootSelect=document.createElement('select')",
		"rootSelect.className='tasks-editor-root'",
		"Discard unsaved tasks.json changes before switching workspace root?",
		"body:JSON.stringify({path:currentTasksPath(),content,expected_sha256:fileMeta.sha256})",
		"fileMeta={path,content,sha256:'',missing:true}",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("taskseditor.js missing workspace-root behavior %q", want)
		}
	}
}

func TestTasksEditorSupportsWorkflowDAGFieldsAndPreview(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/taskseditor.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"function ensureWorkflowMeta(task)",
		"function renderWorkflowFields(form,task)",
		"WORKFLOW / DAG",
		"addField(form,'Depends on'",
		"Parallel dependencies",
		"Sequential dependencies",
		"addField(form,'Retry count'",
		"addField(form,'Timeout seconds'",
		"addField(form,'Condition'",
		"Continue workflow when this task exits non-zero",
		"Declared outputs",
		"::taskdeck-output name=value",
		"function workflowGraphModel()",
		"Cycle detected at ",
		"function openWorkflowGraphPreview()",
		"graph.textContent='Graph'",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("taskseditor.js missing workflow DAG behavior %q", want)
		}
	}
}
