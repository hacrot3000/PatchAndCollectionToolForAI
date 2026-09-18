package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestExplorerLoadsDirectoriesLazily(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/explorer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"/api/project/tree?path=",
		"async function toggleDirectory(pathValue)",
		"if(!loaded.has(pathValue))",
		"taskmenu:project-file-open-request",
		"localStorage.setItem(storageKey()",
		"loaded=new Map()",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("explorer.js missing %q", want)
		}
	}
	if strings.Contains(js, "recursive") {
		t.Fatal("Explorer must not request recursive project trees")
	}
}

func TestExplorerModuleLoadsBeforeMenus(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	explorer := strings.Index(js, "explorer.js")
	menus := strings.Index(js, "menus.js")
	if explorer < 0 || menus < 0 || explorer > menus {
		t.Fatalf("Explorer must load before menus")
	}
}


func TestExplorerRestoresNestedExpandedDirectories(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/explorer.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"async function restoreExpandedDirectories()",
		"a.split('/').length-b.split('/').length",
		"await loadDirectory(pathValue)",
		"expanded.delete(pathValue)",
		"await restoreExpandedDirectories()",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("explorer nested restore missing %q", want)
		}
	}
	if strings.Contains(js, "if(pathValue.includes('/'))continue") {
		t.Fatal("Explorer restore must not skip nested expanded directories")
	}
}
