package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestConfigBackupUIContract(t *testing.T) {
	data,err:=webassets.Files.ReadFile("featuremods/configbackup.js")
	if err!=nil{t.Fatal(err)}
	js:=string(data)
	for _,want:=range []string{
		"BACKUP / RESTORE CONFIGURATION",
		"Secret values and original secret references are never exported.",
		"environment_profiles:globalThis.TaskMenuEnvProfiles?.list?.()||{}",
		"app.jsonFetch('/api/config-backup'",
		"Export backup",
		"Import backup…",
		"Merge with current configuration",
		"Replace portable configuration",
		"Restore never imports plaintext secrets.",
		"globalThis.TaskMenuEnvProfiles?.replaceAll?.",
		"globalThis.TaskMenuEnvProfiles?.mergeAll?.",
		"globalThis.TaskMenuConfigBackup={open,close,exportBackup",
	}{
		if !strings.Contains(js,want){t.Fatalf("configbackup.js missing %q",want)}
	}
}

func TestConfigBackupModuleLoadedAndPaletteAction(t *testing.T) {
	nextData,err:=webassets.Files.ReadFile("featuremods/next.js")
	if err!=nil{t.Fatal(err)}
	if !strings.Contains(string(nextData),"configbackup.js"){t.Fatal("configbackup.js is not loaded")}
	paletteData,err:=webassets.Files.ReadFile("featuremods/terminalpalette.js")
	if err!=nil{t.Fatal(err)}
	palette:=string(paletteData)
	for _,want:=range []string{
		"Settings: Backup / Restore…",
		"TaskMenuConfigBackup?.open?.()",
		"permissionAllowed('settings.read')",
	}{
		if !strings.Contains(palette,want){t.Fatalf("command palette missing %q",want)}
	}
}

func TestEnvironmentProfilesExposeValidatedBackupImport(t *testing.T) {
	data,err:=webassets.Files.ReadFile("featuremods/envprofiles.js")
	if err!=nil{t.Fatal(err)}
	js:=string(data)
	for _,want:=range []string{
		"function normalizeProfileSet(value)",
		"function replaceAllProfiles(value,{selected=''}={})",
		"function mergeAllProfiles(value,{selected=''}={})",
		"replaceAll:replaceAllProfiles",
		"mergeAll:mergeAllProfiles",
	}{
		if !strings.Contains(js,want){t.Fatalf("envprofiles.js missing backup import contract %q",want)}
	}
}
