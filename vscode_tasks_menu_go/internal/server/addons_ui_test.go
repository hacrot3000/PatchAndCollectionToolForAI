package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestAddonBrowserRegistryExtensionPoints(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/addons.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"function registerAction(addonID,spec)",
		"function registerBackgroundJob(addonID,spec)",
		"function registerPanel(addonID,spec)",
		"function registerContextMenuCommand(addonID,spec)",
		"function registerFilePreviewHandler(addonID,spec)",
		"app.jsonFetch('/api/addons'",
		"app.jsonFetch('/api/addons/'+encodeURIComponent(addonID)",
		"TaskMenuOperationCenter?.begin?.({",
		"source:'addon'",
		"cancel:()=>controller.abort()",
		"globalThis.TaskMenuAddons={",
		"commands:commandItems",
		"contextMenuActions",
		"previewActions",
	} {
		if !strings.Contains(js,want) { t.Fatalf("addons.js missing %q",want) }
	}
}

func TestAddonHooksAvoidHardCodedFeatureModules(t *testing.T) {
	projectData, err := webassets.Files.ReadFile("featuremods/projectfileactions.js")
	if err != nil { t.Fatal(err) }
	project := string(projectData)
	for _, want := range []string{
		"TaskMenuAddons?.previewActions?.(pathValue)",
		"Add-on Preview · ",
		"TaskMenuAddons?.contextMenuActions?.(pathValue,type)",
	} {
		if !strings.Contains(project,want) { t.Fatalf("projectfileactions.js missing add-on hook %q",want) }
	}

	paletteData, err := webassets.Files.ReadFile("featuremods/terminalpalette.js")
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(paletteData),"...(globalThis.TaskMenuAddons?.commands?.()||[])") {
		t.Fatal("Unified Command Palette does not consume add-on commands")
	}

	operationData, err := webassets.Files.ReadFile("featuremods/operationcenter.js")
	if err != nil { t.Fatal(err) }
	operation := string(operationData)
	for _, want := range []string{
		"const source=String(spec.source||'database').trim()||'database'",
		"const local=localOperations.get(operationKey(item))",
	} {
		if !strings.Contains(operation,want) { t.Fatalf("Operation Center missing generic add-on job contract %q",want) }
	}
}

func TestAddonRegistryLoadsBeforeConsumers(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	addons := strings.Index(js,"addons.js")
	palette := strings.Index(js,"terminalpalette.js")
	project := strings.Index(js,"projectfileactions.js")
	if addons < 0 || palette < 0 || project < 0 || addons > palette || addons > project {
		t.Fatalf("addons.js must load before palette/project file consumers: %q",js)
	}
}
