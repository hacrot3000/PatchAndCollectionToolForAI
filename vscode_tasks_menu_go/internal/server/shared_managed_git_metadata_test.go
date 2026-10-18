package server

import (
    "testing"

    "bletonfc/vscode_tasks_menu/internal/config"
)

func TestSharedGitTreeMetadataOmitsManagedFilesAndOIDs(t *testing.T) {
    entries:=[]gitTreeCompareEntry{
        {Path:"src/main.go",Kind:"file",OID:"public-sha",Mode:"100644"},
        {Path:".vscode/vscode_tasks_menu.ini",Kind:"file",OID:"private-sha",Mode:"100600"},
        {Path:"docs/vscode_tasks_menu.private.json",Kind:"file",OID:"legacy-sha",Mode:"100644"},
    }
    kept:=filterManagedGitTreeEntries(entries)
    if len(kept)!=1 || kept[0].Path!="src/main.go" {t.Fatalf("Git tree leaked managed metadata: %+v",kept)}

    parentTree:=[]gitTreeCompareEntry{
        {Path:".vscode",Kind:"tree",OID:"private-folder-tree-oid"},
        {Path:".vscode/tasks.json",Kind:"file",OID:"public-tasks-file"},
        {Path:".vscode/vscode_tasks_menu.ini",Kind:"file",OID:"private-config-oid"},
    }
    browsable:=filterManagedGitTreeEntries(parentTree)
    if len(browsable)!=2||browsable[0].Path!=".vscode"||browsable[0].OID!=""||
        browsable[1].Path!=".vscode/tasks.json" {
        t.Fatalf("protected parent tree OID leaked or ordinary file disappeared: %+v",browsable)
    }

    changes:=[]gitTreeDiffChange{
        {Path:"src/main.go",Right:&gitTreeCompareEntry{Path:"src/main.go",OID:"public-sha"}},
        {Path:".vscode/vscode_tasks_menu.ini",Right:&gitTreeCompareEntry{Path:".vscode/vscode_tasks_menu.ini",OID:"private-sha"}},
    }
    safeChanges:=filterManagedGitDiffChanges(changes)
    if len(safeChanges)!=1||safeChanges[0].Path!="src/main.go" {
        t.Fatalf("Git tree diff leaked protected paths: %+v",safeChanges)
    }
}

func TestSharedGitCommitMetadataExcludesRenamesOfManagedFiles(t *testing.T) {
    items:=[]gitGraphFile{
        {Status:"A",Path:"src/main.go"},
        {Status:"M",Path:".vscode/vscode_tasks_menu.ini"},
        {Status:"R100",Path:"public-name.txt",OldPath:"vscode_tasks_menu.private.json"},
        {Status:"R100",Path:"vscode_tasks_menu.secret.json",OldPath:"public-name.txt"},
    }
    got:=filterManagedGitGraphFiles(items)
    if len(got)!=1||got[0].Path!="src/main.go"{
        t.Fatalf("Git commit metadata exposed internal rename paths: %+v",got)
    }
    raw:="A\x00src/main.go\x00M\x00.vscode/vscode_tasks_menu.ini\x00"
    cfg:=config.Default()
    cfg.SharedServerEnabled=true
    s:=&Server{Config:cfg}
    shared:=s.sharedGitGraphFiles(raw)
    if len(shared)!=1||shared[0].Path!="src/main.go"{
        t.Fatalf("shared Git API did not redact private paths: %+v",shared)
    }
    cfg.SharedServerEnabled=false
    s.Config=cfg
    local:=s.sharedGitGraphFiles(raw)
    if len(local)!=2{
        t.Fatalf("single-user Git metadata unexpectedly restricted: %+v",local)
    }
}

func TestSharedGitStatusMetadataRedactsRenamedManagedPaths(t *testing.T) {
    // --porcelain=v1 -z exposes both the current and original rename paths.
    raw:=" M src/main.go\x00?? .vscode/vscode_tasks_menu.ini\x00R  harmless.txt\x00vscode_tasks_menu.secrets.json\x00"
    changes:=parseGitStatusZ(raw)
    if len(changes)!=3 {t.Fatalf("fixture parse failed: %+v",changes)}
    got:=filterManagedGitStatusChanges(changes)
    if len(got)!=1||got[0].Path!="src/main.go" {
        t.Fatalf("managed-state filename or rename leaked: %+v",got)
    }
    for _,item:=range got {
        if isTaskDeckManagedProjectPath(item.Path)||isTaskDeckManagedProjectPath(item.OriginalPath){
            t.Fatalf("protected Git status path exposed: %+v",item)
        }
    }
    cfg:=config.Default()
    cfg.SharedServerEnabled=true
    s:=&Server{Config:cfg}
    if !s.Config.SharedServerEnabled{t.Fatal("shared fixture not enabled")}
    // Classification and filtering are shared-mode only at the HTTP edge;
    // local single-user Git status still uses the original parse output.
    if len(changes)!=3{t.Fatal("raw single-user output unexpectedly altered")}
}

func TestSharedGitConflictMetadataExcludesProtectedPaths(t *testing.T) {
    conflicts:=[]gitConflictFile{
        {Path:"src/app.go",ProjectPath:"src/app.go",Status:"UU"},
        {Path:".vscode/vscode_tasks_menu.ini",ProjectPath:".vscode/vscode_tasks_menu.ini",Status:"AA"},
        {Path:"safe-alias.txt",ProjectPath:"vscode_tasks_menu.private.ini",Status:"UU"},
    }
    got:=filterManagedGitConflictFiles(conflicts)
    if len(got)!=1||got[0].Path!="src/app.go"{
        t.Fatalf("shared Git conflict source leaked protected path: %+v",got)
    }
    if len(conflicts)!=3{t.Fatal("filter unexpectedly changed original state")}
}
