package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestProjectProfileFrontendCapabilityAdapters(t *testing.T) {
	tests := []struct{ file string; required []string }{
		{file:"featuremods/envprofiles.js",required:[]string{
			"globalThis.TaskMenuEnvProfiles={",
			"currentName,",
			"currentEnv",
			"applySnapshot,",
			"clearOverride,",
			"get(name)",
			"Object.prototype.hasOwnProperty.call(readProfiles(),name)",
			"setSelected(name)",
		}},
		{file:"featuremods/commandpresets.js",required:[]string{
			"let projectPresetIDs=[]",
			"function selectedProjectPresets()",
			"function setProjectPresetIDs(ids)",
			"taskmenu:project-command-presets",
			"setProjectPresetIDs,",
			"get projectPresetIDs(){return [...projectPresetIDs];}",
		}},
		{file:"featuremods/all.js",required:[]string{
			"let projectProfileTaskIDs=[]",
			"◆ PROFILE TASKS",
			"globalThis.TaskMenuTaskSet={",
			"projectProfileTaskIDs=[...new Set",
			"get ids(){return [...projectProfileTaskIDs];}",
		}},
		{file:"featuremods/connections.js",required:[]string{
			"async openLocal(cwd='.')",
			"async openSSHProfile(id)",
			"const profile=sshProfiles.find(item=>item.id===id)",
			"get sshProfiles(){return [...sshProfiles];}",
		}},
	}
	for _, tc := range tests {
		data, err := webassets.Files.ReadFile(tc.file)
		if err != nil { t.Fatal(err) }
		js := string(data)
		for _, want := range tc.required {
			if !strings.Contains(js,want) { t.Fatalf("%s missing project profile adapter %q",tc.file,want) }
		}
	}
}

func TestProjectProfilesManagerAndApplyUI(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/projectprofiles.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"const endpoint='/api/project-profiles'",
		"async function loadProfiles()",
		"function currentDraft(name='')",
		"function projectRelativeCwd(value)",
		"environment_profile:String(globalThis.TaskMenuEnvProfiles?.currentName?.()||'')",
		"environment:{...(globalThis.TaskMenuEnvProfiles?.currentEnv?.()||{})}",
		"command_preset_ids:uniqueStrings(globalThis.TaskMenuCommandPresets?.projectPresetIDs||[])",
		"task_ids:uniqueInts(globalThis.TaskMenuTaskSet?.ids||[])",
		"async function applyProfile(profile,{confirmOpen=true}={})",
		"No task or command preset will run automatically.",
		"TaskMenuEnvProfiles?.applySnapshot?.",
		"const envSnapshot=profile.environment",
		"saved snapshot",
		"TaskMenuEnvProfiles?.get?.(env.value)",
		"environment:{...(localEnv||savedEnv)}",
		"do not put passwords/tokens in environment values",
		"TaskMenuCommandPresets?.setProjectPresetIDs?.",
		"TaskMenuTaskSet?.apply?.",
		"TaskMenuGitFiles?.restoreState?.",
		"TaskMenuConnections?.openSSHProfile?.",
		"TaskMenuDatabase?.openProfile?.",
		"TaskMenuFileTransfer?.openProfile?.",
		"async function createLocalTerminal(spec)",
		"New from current",
		"Command preset set (selected only; never auto-run)",
		"Task set (quick access; never auto-run)",
		"Project profiles store only references to existing connection profiles.",
		"globalThis.TaskMenuProjectProfiles=",
	} {
		if !strings.Contains(js,want) { t.Fatalf("projectprofiles.js missing %q",want) }
	}
	for _, forbidden := range []string{"private_key_passphrase","password_value","secret_ref"} {
		if strings.Contains(js,forbidden) { t.Fatalf("projectprofiles.js unexpectedly references secret field %q",forbidden) }
	}
	if strings.Contains(js,"innerHTML") || strings.Contains(js,"eval(") || strings.Contains(js,"new Function(") {
		t.Fatal("project profile UI must not use dynamic HTML/eval")
	}
}

func TestProjectProfilesModuleLoadOrder(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	snapshots := strings.Index(js,"featuremods/workspacesnapshots.js")
	profiles := strings.Index(js,"featuremods/projectprofiles.js")
	if snapshots < 0 || profiles < 0 || profiles < snapshots {
		t.Fatalf("load order snapshots=%d project profiles=%d",snapshots,profiles)
	}
}
