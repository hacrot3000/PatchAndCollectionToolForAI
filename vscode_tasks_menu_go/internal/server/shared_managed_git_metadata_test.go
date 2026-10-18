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
