package server

import (
    "context"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
    "testing"

    "bletonfc/vscode_tasks_menu/internal/identity"
    "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func requireGit(t *testing.T) {
    t.Helper()
    if _,err:=exec.LookPath("git");err!=nil { t.Skip("git executable not installed") }
}

func gitInitRemoteTest(t *testing.T, root string) {
    t.Helper()
    command:=exec.Command("git","-C",root,"init","-q")
    if output,err:=command.CombinedOutput();err!=nil{
        t.Fatalf("create test Git repo: %v: %s",err,output)
    }
}

func TestRemoteGitStatusHostRBACAndEncryptedRoundTrip(t *testing.T) {
    requireGit(t)
    s, session, client, agent := remoteDispatchFixture(t)
    gitInitRemoteTest(t, s.Workspace)
    request,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
        RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionGitStatus,
    })
    if err!=nil {t.Fatal(err)}
    reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,request)
    if err!=nil {t.Fatal(err)}
    status,err:=remotetransport.OpenRemoteActionGitStatusResult(client,reply,remoteDispatchTestID)
    if err!=nil {t.Fatal(err)}
    if !strings.Contains(status,"## ") || strings.Contains(status, "\x1b") {
        t.Fatalf("unexpected decrypted repo status: %q",status)
    }
    project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
    if err!=nil {t.Fatal(err)}
    audit,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
        ProjectID:project.ID,Action:"remote.action.git_status",Limit:10,
    })
    if err!=nil || len(audit)!=1 || audit[0].UserID==nil ||
        *audit[0].UserID!=session.userID || audit[0].Result!="authorized" {
        t.Fatalf("git.status not audited under real TaskDeck principal: %+v, %v",audit,err)
    }
    if _,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,request);err==nil {
        t.Fatal("replayed Git status request was accepted")
    }
}

func TestRemoteGitStatusCannotSelectParentRepository(t *testing.T) {
    requireGit(t)
    root:=t.TempDir()
    gitInitRemoteTest(t,root)
    nested:=filepath.Join(root,"nested")
    if err:=os.MkdirAll(nested,0700);err!=nil{t.Fatal(err)}
    s:=&Server{Workspace:nested}
    if _,err:=s.readRemoteGitStatus(context.Background());err==nil {
        t.Fatal("remote Git status walked outside TaskDeck workspace root")
    }
}

func TestRemoteGitStatusRejectsUnapprovedClientIdentity(t *testing.T) {
    requireGit(t)
    s,session,client,agent:=remoteDispatchFixture(t)
    gitInitRemoteTest(t,s.Workspace)
    session.clientProofVerified=false
    request,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
        RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionGitStatus,
    })
    if err!=nil{t.Fatal(err)}
    reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,request)
    if err==nil || len(reply)>0 {t.Fatal("PHP password+ticket without host-approved key read Git state")}
}
