package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestProjectHealthDashboardAggregatesExistingSubsystems(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/projecthealth.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"PROJECT HEALTH",
		"/api/git/status",
		"/api/task-runs",
		"/api/project/health",
		"TaskMenuOperationCenter?.refresh?.()",
		"TaskMenuConnectionGraph?.refresh?.()",
		"Running tasks",
		"Open terminals",
		"Failing transfers",
		"DB connections",
		"SSH status",
		"Project disk usage",
		"Last build",
		"Last task",
		"TaskMenuProjectHealth={open,close,refresh,collect",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("projecthealth.js missing %q", want)
		}
	}
	if strings.Contains(js, "/api/project/tree") || strings.Contains(js, "/api/project/files/search") {
		t.Fatal("Project Health must not crawl the project tree from the browser")
	}
}

func TestProjectHealthLauncherAndModuleOrder(t *testing.T) {
	nextData, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil { t.Fatal(err) }
	next := string(nextData)
	operations := strings.Index(next, "operationcenter.js")
	graph := strings.Index(next, "connection_graph.js")
	health := strings.Index(next, "projecthealth.js")
	if operations < 0 || graph < 0 || health < 0 || health < operations || health < graph {
		t.Fatalf("projecthealth.js must load after Operation Center and Connection Graph: %q", next)
	}

	paletteData, err := webassets.Files.ReadFile("featuremods/terminalpalette.js")
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(paletteData), "Project: Health Dashboard…") {
		t.Fatal("Unified Command Palette does not expose Project Health")
	}
}
