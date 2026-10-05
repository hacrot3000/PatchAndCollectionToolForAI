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
			"currentName:selectedName",
			"currentEnv",
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
