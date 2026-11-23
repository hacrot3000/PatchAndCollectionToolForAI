package server

import (
 "context"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "time"

 "bletonfc/vscode_tasks_menu/internal/agentchat"
)

func TestAgentOwnQueuedZipDoesNotInvalidateApprovalButOtherDriftDoes(t *testing.T) {
 s,id,_:=agentPendingCallFixture(t,true)
 gitQuickRun(t,s.Workspace,"init","-q")
 gitQuickRun(t,s.Workspace,"config","user.email","agent@example.invalid")
 gitQuickRun(t,s.Workspace,"config","user.name","Agent Test")
 if err:=os.WriteFile(filepath.Join(s.Workspace,"tracked.txt"),[]byte("baseline\n"),0600);err!=nil{t.Fatal(err)}
 gitQuickRun(t,s.Workspace,"add","tracked.txt")
 gitQuickRun(t,s.Workspace,"commit","-q","-m","baseline")
 snap,err:=agentWorkspaceSnapshotOf(context.Background(),s.Workspace)
 if err!=nil{t.Fatal(err)}
 store,err:=s.agentChatStore();if err!=nil{t.Fatal(err)}
 x,err:=store.Get(id,"local");if err!=nil{t.Fatal(err)}
 call:=agentchat.ToolCallRecord{
    ExpectedHead:snap.Head,ExpectedBranch:snap.Branch,
    WorkspaceStatusSHA256:snap.StatusSHA256,GitIsRepo:true,
    ApprovalExpiresAt:func()*time.Time{t:=time.Now().UTC().Add(time.Minute);return &t}(),
 }
 queue:="patchs/PATCH_CLEAR_TEXT_agent-created-test.zip"
 if err:=os.MkdirAll(filepath.Join(s.Workspace,"patchs"),0700);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(s.Workspace,filepath.FromSlash(queue)),[]byte("self-generated queue package"),0600);err!=nil{t.Fatal(err)}
 if err=s.agentVerifyApprovalStillValid(context.Background(),&call);err==nil{
  t.Fatal("normal drift check unexpectedly ignored the new queue file")
 }
 if err=s.agentVerifyApprovalAfterOwnQueue(context.Background(),&call,queue);err!=nil{
  t.Fatalf("exact queued ZIP should not count as unrelated source drift: %v",err)
 }
 for _,invalid:=range []string{"../tracked.txt","patchs/other/patch.zip","patchs/../tracked.txt","patchs/a.txt"}{
  if err=s.agentVerifyApprovalAfterOwnQueue(context.Background(),&call,invalid);err==nil {
   t.Fatalf("invalid/unmatched queue exception accepted: %q",invalid)
  }
 }
 if err=os.WriteFile(filepath.Join(s.Workspace,"tracked.txt"),[]byte("external edit\n"),0600);err!=nil{t.Fatal(err)}
 if err=s.agentVerifyApprovalAfterOwnQueue(context.Background(),&call,queue);err==nil{
  t.Fatal("tracked file drift was hidden by queue exception")
 }
 if err=os.WriteFile(filepath.Join(s.Workspace,"tracked.txt"),[]byte("baseline\n"),0600);err!=nil{t.Fatal(err)}
 if err=os.WriteFile(filepath.Join(s.Workspace,"unrelated-new-file.txt"),[]byte("extra"),0600);err!=nil{t.Fatal(err)}
 if err=s.agentVerifyApprovalAfterOwnQueue(context.Background(),&call,queue);err==nil{
  t.Fatal("untracked user file drift was hidden by queue exception")
 }
 if err=os.Remove(filepath.Join(s.Workspace,"unrelated-new-file.txt"));err!=nil{t.Fatal(err)}
 gitQuickRun(t,s.Workspace,"checkout","-q","-b","unrelated")
 if err=s.agentVerifyApprovalAfterOwnQueue(context.Background(),&call,queue);err==nil{
  t.Fatal("branch change was hidden by queue exception")
 }
 if !strings.HasPrefix(x.Status,"waiting_tool"){t.Fatalf("fixture unexpected status: %s",x.Status)}
}
