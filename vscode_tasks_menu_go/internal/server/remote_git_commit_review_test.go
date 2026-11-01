package server

import (
 "bytes"
 "context"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func TestGitCommitReviewHostReadOnlyShowsEntireStagedPatch(t *testing.T){
 s,session,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleAdmin)
 if err:=exec.Command("git","init","-q",s.Workspace).Run();err!=nil{t.Skipf("Git unavailable: %v",err)}
 path:=filepath.Join(s.Workspace,"code.txt")
 if err:=os.WriteFile(path,[]byte("original\n"),0600);err!=nil{t.Fatal(err)}
 for _,args:=range [][]string{
  {"-C",s.Workspace,"add","--","code.txt"},
  {"-C",s.Workspace,"-c","user.name=TaskDeck CI","-c",
   "user.email=ci@example.invalid","commit","-qm","baseline"},
 }{
  if result,err:=exec.Command("git",args...).CombinedOutput();err!=nil{t.Fatalf("%s %v",result,err)}
 }
 originalHead,err:=exec.Command("git","-C",s.Workspace,"rev-parse","HEAD").Output()
 if err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(path,[]byte("staged bytes\n"),0600);err!=nil{t.Fatal(err)}
 if result,err:=exec.Command("git","-C",s.Workspace,"add","--","code.txt").CombinedOutput();err!=nil{
  t.Fatalf("git add fixture: %s %v",result,err)
 }
 indexPath:=filepath.Join(s.Workspace,".git","index")
 oldIndex,err:=os.ReadFile(indexPath)
 if err!=nil{t.Fatal(err)}
 // Independently modify the worktree after staging; preview must
 // display exactly the STAGED snapshot rather than the current bytes.
 if err:=os.WriteFile(path,[]byte("unstaged bytes\n"),0600);err!=nil{t.Fatal(err)}
 request,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionGitCommitReview,
 })
 if err!=nil{t.Fatal(err)}
 if reply,err:=s.executeRemoteE2EEPing(context.Background(),nil,agent,request);err==nil||len(reply)!=0{
  t.Fatal("compromised PHP granted commit preview without TaskDeck principal")
 }
 reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,request)
 if err!=nil{t.Fatal(err)}
 review,err:=remotetransport.OpenRemoteGitCommitReviewResult(client,reply,remoteDispatchTestID)
 if err!=nil||!strings.Contains(review,"HEAD "+strings.TrimSpace(string(originalHead)))||
  !strings.Contains(review,"BRANCH refs/heads/")||
  !strings.Contains(review,"INDEX_SHA256 ")||
  !strings.Contains(review,"+staged bytes")||
  strings.Contains(review,"+unstaged bytes"){
  t.Fatalf("commit review was not complete/staged-only: %q %v",review,err)
 }
 nowIndex,err:=os.ReadFile(indexPath)
 if err!=nil||!bytes.Equal(oldIndex,nowIndex){t.Fatal("read-only commit preview mutated index")}
 headAfter,err:=exec.Command("git","-C",s.Workspace,"rev-parse","HEAD").Output()
 if err!=nil||!bytes.Equal(originalHead,headAfter){t.Fatal("commit preview changed HEAD")}
 if _,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,request);err==nil{
  t.Fatal("replayed E2EE commit preview was accepted")
 }
}

func TestGitCommitReviewHostRejectsEmptyIndexAndViewer(t *testing.T){
 for _,role:=range []remotetransport.Role{remotetransport.RoleAdmin,remotetransport.RoleViewer}{
  t.Run(string(role),func(t *testing.T){
   s,session,client,agent:=remoteDispatchFixtureWithRole(t,role)
   if err:=exec.Command("git","init","-q",s.Workspace).Run();err!=nil{t.Skipf("git unavailable: %v",err)}
   path:=filepath.Join(s.Workspace,"code.txt")
   if err:=os.WriteFile(path,[]byte("baseline\n"),0600);err!=nil{t.Fatal(err)}
   for _,args:=range [][]string{
    {"-C",s.Workspace,"add","--","code.txt"},
    {"-C",s.Workspace,"-c","user.name=TaskDeck CI","-c",
     "user.email=ci@example.invalid","commit","-qm","baseline"},
   }{
    if output,err:=exec.Command("git",args...).CombinedOutput();err!=nil{t.Fatalf("fixture %v %s",err,output)}
   }
   if role==remotetransport.RoleViewer{
    if err:=os.WriteFile(path,[]byte("staged\n"),0600);err!=nil{t.Fatal(err)}
    if output,err:=exec.Command("git","-C",s.Workspace,"add","--","code.txt").CombinedOutput();err!=nil{t.Fatalf("fixture %v %s",err,output)}
   }
   request,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
    RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionGitCommitReview,
   })
   if err!=nil{t.Fatal(err)}
   if reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,request);err==nil||len(reply)!=0{
    t.Fatalf("empty staged/unauthorized viewer preview accepted: %s",role)
   }
  })
 }
}

func TestGitCommitReviewRejectsExecutableFilterAndTruncation(t *testing.T){
 for _,danger:=range []string{"filter","oversize"} {
  t.Run(danger,func(t *testing.T){
   s,session,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleAdmin)
   if err:=exec.Command("git","init","-q",s.Workspace).Run();err!=nil{t.Skipf("git unavailable: %v",err)}
   data:=[]byte("initial\n")
   path:=filepath.Join(s.Workspace,"code.txt")
   if err:=os.WriteFile(path,data,0600);err!=nil{t.Fatal(err)}
   for _,args:=range [][]string{
    {"-C",s.Workspace,"add","--","code.txt"},
    {"-C",s.Workspace,"-c","user.name=TaskDeck CI","-c",
     "user.email=ci@example.invalid","commit","-qm","baseline"},
   }{
    if output,err:=exec.Command("git",args...).CombinedOutput();err!=nil{t.Fatalf("fixture %v %s",err,output)}
   }
   if danger=="filter"{
    // Command is intentionally never executed.
    if output,err:=exec.Command("git","-C",s.Workspace,"config",
     "filter.evil.clean","sh -c 'exit 88'").CombinedOutput();err!=nil{
     t.Fatalf("fixture %v %s",err,output)
    }
   }
   if danger=="oversize"{
    data=[]byte(strings.Repeat("many staged lines\n",400))
   }else{data=[]byte("new\n")}
   if err:=os.WriteFile(path,data,0600);err!=nil{t.Fatal(err)}
   if output,err:=exec.Command("git","-C",s.Workspace,"add","--","code.txt").CombinedOutput();err!=nil{
    t.Fatalf("fixture %v %s",err,output)
   }
   request,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
    RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionGitCommitReview,
   })
   if err!=nil{t.Fatal(err)}
   if reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,request);err==nil||len(reply)!=0{
    t.Fatalf("dangerous/oversize Git commit preview accepted: %s",danger)
   }
  })
 }
}
