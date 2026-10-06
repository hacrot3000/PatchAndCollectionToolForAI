package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestRemoteWorkspaceBrowserIntegration(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/remote_workspace.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"REMOTE WORKSPACE",
		"/api/remote-workspaces",
		"/api/remote-workspace-files",
		"/api/remote-workspace-git",
		"operation:'read'",
		"operation:'list'",
		"TaskMenuEditor?.openDocument?.(",
		"remote_workspace_id:String(workspace.id)",
		"remote_path:String(relative)",
		"remote_cwd:remoteCwd",
		"ssh_profile_id:workspace.ssh_profile_id",
		"TaskMenuDatabase?.openProfile?.(id)",
		"database_profile_ids",
		"function remoteCanRead()",
		"function remoteCanWrite()",
		"read_only:!remoteCanWrite()",
		"function gitAllowed(action)",
		"Git on remote host",
		"Terminal here",
		"globalThis.TaskMenuRemoteWorkspace={",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("remote_workspace.js missing %q", want)
		}
	}
}

func TestRemoteWorkspaceEditorUsesOptimisticRemoteTransport(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/editor.js")
	if err != nil { t.Fatal(err) }
	js := string(data)
	for _, want := range []string{
		"if(view.file?.remote_workspace_id)",
		"/api/remote-workspace-files",
		"operation:'write'",
		"operation:'read'",
		"expected_sha256:expectedSHA256",
		"function remoteEditorSerializedText(view)",
		"function remoteEditorFileFromResponse(base,data)",
		"async function openDocument(file)",
		"openDocument,",
		"if(view?.file?.remote_workspace_id)return;",
		"filter(view=>!view.closed&&!view.file?.remote_workspace_id)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("editor.js missing Remote Workspace editor contract %q", want)
		}
	}
}

func TestRemoteWorkspaceDoesNotLeakIntoLocalOnlyServices(t *testing.T) {
	watcherData, err := webassets.Files.ReadFile("featuremods/filewatcher.js")
	if err != nil { t.Fatal(err) }
	watcher := string(watcherData)
	for _, want := range []string{
		"view.file?.remote_workspace_id",
		"!view.file?.remote_workspace_id",
	} {
		if !strings.Contains(watcher, want) {
			t.Fatalf("filewatcher.js missing remote exclusion %q", want)
		}
	}

	lspData, err := webassets.Files.ReadFile("featuremods/lsp.js")
	if err != nil { t.Fatal(err) }
	lsp := string(lspData)
	for _, want := range []string{
		"Language Server actions are not available for Remote Workspace files",
		"if(view.file?.remote_workspace_id)return;",
	} {
		if !strings.Contains(lsp, want) {
			t.Fatalf("lsp.js missing remote exclusion %q", want)
		}
	}

	quickData, err := webassets.Files.ReadFile("featuremods/quickopen.js")
	if err != nil { t.Fatal(err) }
	quick := string(quickData)
	for _, want := range []string{
		"const remote=Boolean(view.file?.remote_workspace_id)",
		"Remote Editor · ",
		"TaskMenuEditor?.activateEditor?.(view.id)",
	} {
		if !strings.Contains(quick, want) {
			t.Fatalf("quickopen.js missing remote editor tab behavior %q", want)
		}
	}
}

func TestRemoteWorkspaceModuleIsReachableFromMainUI(t *testing.T) {
	nextData, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil { t.Fatal(err) }
	if !strings.Contains(string(nextData), "import '/featuremods/remote_workspace.js'") {
		t.Fatal("Remote Workspace module is not loaded")
	}

	connectionsData, err := webassets.Files.ReadFile("featuremods/connections.js")
	if err != nil { t.Fatal(err) }
	connections := string(connectionsData)
	for _, want := range []string{
		"remoteWorkspace.textContent='Remote'",
		"app.hasPermission?.('ssh.use')&&app.hasPermission?.('settings.read')",
		"TaskMenuRemoteWorkspace?.open?.()",
	} {
		if !strings.Contains(connections, want) {
			t.Fatalf("Connections missing Remote Workspace launcher %q", want)
		}
	}

	paletteData, err := webassets.Files.ReadFile("featuremods/terminalpalette.js")
	if err != nil { t.Fatal(err) }
	palette := string(paletteData)
	for _, want := range []string{
		"Remote Workspace: Open…",
		"TaskMenuRemoteWorkspace?.open?.()",
		"permissionAllowed('ssh.use')",
		"permissionAllowed('settings.read')",
	} {
		if !strings.Contains(palette, want) {
			t.Fatalf("Command Palette missing Remote Workspace action %q", want)
		}
	}
}
