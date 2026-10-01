package server

import (
	"strings"
	"testing"

	webassets "bletonfc/vscode_tasks_menu/web"
)

func TestConnectionsPanelUsesExistingTerminalSessionsAndSafeProfileAPI(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/connections.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"task-connections-panel",
		"app.jsonFetch('/api/ssh/profiles')",
		"app.jsonFetch('/api/db/adapters')",
		"app.jsonFetch('/api/db/profiles')",
		"section('DATABASES'",
		"openDatabaseProfileDialog",
		"TaskMenuDatabase",
		"read_only",
		"transport.input.value==='ssh_tunnel'",
		"payload.ssh_profile_id=sshProfile.input.value",
		"Select an SSH profile for the database tunnel",
		"SSH tunnel",
		"Clear saved password",
		"kind==='redis'",
		"command_timeout_seconds",
		"Database index",
		"databaseDefaultPort(kind)",
		"databaseDefaultPort(adapterKind)",
		"kind==='mongo'",
		"return 27017",
		"Authentication database",
		"Use TLS",
		"options.auth_source",
		"options.tls='true'",
		"kind==='sqlite'",
		"SQLite database file",
		"Busy timeout (ms)",
		"options.busy_timeout_ms",
		"timeout.wrap.style.display=sqlite?'none':'flex'",
		"adapterKind!=='sqlite'&&timeout.input.value.trim()",
		"transport.wrap.style.display=sqlite?'none':'flex'",
		"file:sqlite?sqliteFile.input.value:''",
		"port:sqlite?0:",
		"if(kind==='sqlite')return 0",
		"if(kind==='sqlite')return (profile?.file||'SQLite file')",
		"app.jsonFetch('/api/ssh/test'",
		"async function runSSHTest(payload,button)",
		"body:JSON.stringify(payload)",
		"return runSSHTest({profile_id:profile.id},button)",
		"function currentProfilePayload()",
		"const test=document.createElement('button')",
		"const payload={profile:currentProfilePayload()}",
		"if(editing)payload.profile_id=profile.id",
		"Test current edits without saving",
		"Test connection before adding this profile",
		"function showConnectionContextMenu(items,x,y)",
		"row.oncontextmenu=event=>",
		"label:'Clone'",
		"label:'Delete'",
		"label:'Test'",
		"label:'Edit'",
		"onClone:()=>openProfileDialog(clonedProfile(profile))",
		"onTest:()=>testSSHFromMenu(profile)",
		"onClone:()=>openDatabaseProfileDialog(clonedProfile(profile))",
		"onTest:()=>testDatabaseFromMenu(profile)",
		"open.onclick=()=>Promise.resolve(onOpen()).catch(app.showError)",
		"function currentDatabaseProfilePayload()",
		"const test=editing?document.createElement('button'):null",
		"testDatabaseDraft(profile.id,currentDatabaseProfilePayload(),test)",
		"Test current edits without saving",
		"if(test)actions.append(test)",
		"Test connection",
		"app.jsonFetch('/api/config/terminal-cwds')",
		"body:JSON.stringify({kind:'terminal',...payload})",
		"app.materializeSession(meta,true)",
		"ssh_profile_id:profile.id",
		"cwd",
		"Custom remote home directory",
		"Preset commands",
		"ProxyJump",
		"Password / passphrase",
		"payload.secret=secret.input.value",
		"has_secret",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("connections.js missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"profile.secret_ref",
		"profile.secret",
		"innerHTML=profile",
		"task-connection-actions",
	} {
		if strings.Contains(js, forbidden) {
			t.Fatalf("connections.js must not expose stored SSH secrets through %q", forbidden)
		}
	}
}


func TestConnectionsContextMenuCloneDoesNotExposeStoredSecrets(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/connections.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"function clonedProfile(profile)",
		"delete clone.id",
		"delete clone.has_secret",
		"+' Copy'",
		"onDelete:()=>deleteProfile(profile)",
		"onEdit:()=>openProfileDialog(profile)",
		"onDelete:()=>deleteDatabaseProfile(profile)",
		"onEdit:()=>openDatabaseProfileDialog(profile)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("connections.js missing context-menu clone behavior %q", want)
		}
	}
}

func TestConnectionsPanelIsLoadedBeforeActivityBar(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/next.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	connections := strings.Index(js, "connections.js")
	activity := strings.Index(js, "activitybar.js")
	if connections < 0 || activity < 0 || connections > activity {
		t.Fatalf("Connections must load before Activity Bar: connections=%d activity=%d", connections, activity)
	}
}

func TestActivityBarIntegratesConnectionsView(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/activitybar.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"makeButton('connections','Connections'",
		"function showConnections()",
		"globalThis.TaskMenuConnections?.open()",
		"globalThis.TaskMenuConnections?.close()",
		"activeView==='connections'",
		"TaskMenuConnections?.panel?.contains(target)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("activitybar.js missing Connections integration %q", want)
		}
	}
}

func TestTerminalCWDInjectionPreservesExplicitConnectionTarget(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/terminalcwd.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"!payload.ssh_profile_id",
		"!Object.prototype.hasOwnProperty.call(payload,'cwd')",
		"cwd:selectedCWD()",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("terminalcwd.js missing explicit-target protection %q", want)
		}
	}
}


func TestConnectionsPanelTracksSharedHeaderHeight(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/connections.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		".task-connections-panel{display:none;position:fixed;top:var(--taskmenu-header-height,30px)",
		"body:not(.task-sidebar-auto-hide) .task-connections-panel{top:calc(var(--taskmenu-header-height,30px) + 46px)",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("Connections compact header offset missing %q", want)
		}
	}


func TestConnectionsUIIncludesFTPAndSFTPProfiles(t *testing.T) {
	data, err := webassets.Files.ReadFile("featuremods/connections.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{
		"/api/file-transfer/profiles",
		"openFileTransferProfileDialog",
		"protocol==='sftp'",
		"protocol==='ftp'",
		"FTP is not encrypted",
		"TaskMenuFileTransfer",
		"testFileTransferDraft",
	} {
		if !strings.Contains(js, want) {
			t.Fatalf("connections.js missing file-transfer UI contract %q", want)
		}
	}
}
}
