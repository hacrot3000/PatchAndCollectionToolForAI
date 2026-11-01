package server

import (
 "bytes"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func remoteGitUnstageFixture(t *testing.T)(*Server,*remoteTaskDeckSession,*remotetransport.Channel,*remotetransport.Channel){
 t.Helper()
 s,session,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleAdmin)
 if err:=exec.Command("git","init","-q",s.Workspace).Run();err!=nil{
  t.Skipf("Git unavailable: %v",err)
 }
 t.Setenv("TASKDECK_REMOTE_GIT_WRITE","1")
 t.Setenv("TASKDECK_REMOTE_GIT_UNSTAGE","1")
 return s,session,client,agent
}

func remoteUnstageSetupBase(t *testing.T,root string){
 t.Helper()
 for _,args:=range [][]string{
  {"-C",root,"add","--","source.txt"},
  {"-C",root,"-c","user.name=TaskDeck CI","-c",
   "user.email=ci@example.invalid","commit","-qm","baseline"},
 }{
  if out,err:=exec.Command("git",args...).CombinedOutput();err!=nil{
   t.Fatalf("Git fixture %v: %v %s",args,err,out)
  }
 }
}
func remoteUnstageGitCommand(t *testing.T,root string,args ...string)[]byte{
 t.Helper()
 out,err:=exec.Command("git",append([]string{"-C",root},args...)...).CombinedOutput()
 if err!=nil{t.Fatalf("Git %v failed: %v %s",args,err,out)}
 return out
}
func remoteUnstageRequest(t *testing.T,client *remotetransport.Channel,root string,expectedOverride string)[]byte{
 t.Helper()
 diff:=remoteUnstageGitCommand(t,root,"--no-pager","diff","--cached","--no-ext-diff",
  "--no-textconv","--no-color","--","source.txt")
 sum:=sha256.Sum256(diff)
 expected:=hex.EncodeToString(sum[:])
 if expectedOverride!=""{expected=expectedOverride}
 request,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionGitUnstage,
  Path:"source.txt",ExpectedSHA256:expected,
 })
 if err!=nil{t.Fatal(err)}
 return request
}

func TestGitUnstageHostRestoresHEADOnlyForSelectedFile(t *testing.T){
 s,session,client,agent:=remoteGitUnstageFixture(t)
 path:=filepath.Join(s.Workspace,"source.txt")
 if err:=os.WriteFile(path,[]byte("base\n"),0600);err!=nil{t.Fatal(err)}
 remoteUnstageSetupBase(t,s.Workspace)
 if err:=os.WriteFile(path,[]byte("staged\n"),0600);err!=nil{t.Fatal(err)}
 remoteUnstageGitCommand(t,s.Workspace,"add","--","source.txt")
 other:=filepath.Join(s.Workspace,"other.txt")
 if err:=os.WriteFile(other,[]byte("staged other\n"),0600);err!=nil{t.Fatal(err)}
 remoteUnstageGitCommand(t,s.Workspace,"add","--","other.txt")
 if err:=os.WriteFile(path,[]byte("worktree after stage\n"),0600);err!=nil{t.Fatal(err)}
 request:=remoteUnstageRequest(t,client,s.Workspace,"")
 if reply,err:=s.executeRemoteE2EEPing(context.Background(),nil,agent,request);err==nil||len(reply)!=0{
  t.Fatal("compromised PHP-only grant bypassed TaskDeck unstage login")
 }
 reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,request)
 if err!=nil{t.Fatal(err)}
 ack,err:=remotetransport.OpenRemoteGitUnstageResult(client,reply,remoteDispatchTestID)
 if err!=nil||ack!="unstaged"{t.Fatalf("unauthenticated/missing unstage result %q %v",ack,err)}
 staged:=remoteUnstageGitCommand(t,s.Workspace,"show",":source.txt")
 if !bytes.Equal(staged,[]byte("base\n")){t.Fatalf("HEAD not restored to index: %q",staged)}
 otherStaged:=remoteUnstageGitCommand(t,s.Workspace,"show",":other.txt")
 if !bytes.Equal(otherStaged,[]byte("staged other\n")){t.Fatal("unstage altered unrelated index file")}
 actual,err:=os.ReadFile(path)
 if err!=nil||!bytes.Equal(actual,[]byte("worktree after stage\n")){
  t.Fatalf("unstage modified working tree: %q %v",actual,err)
 }
 if _,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,request);err==nil{
  t.Fatal("replayed authenticated unstage accepted")
 }
}

func TestGitUnstageHostRemovesOnlyNewFileFromIndex(t *testing.T){
 s,session,client,agent:=remoteGitUnstageFixture(t)
 base:=filepath.Join(s.Workspace,"other.txt")
 if err:=os.WriteFile(base,[]byte("committed\n"),0600);err!=nil{t.Fatal(err)}
 remoteUnstageGitCommand(t,s.Workspace,"add","--","other.txt")
 remoteUnstageGitCommand(t,s.Workspace,"-c","user.name=TaskDeck CI",
  "-c","user.email=ci@example.invalid","commit","-qm","baseline")
 path:=filepath.Join(s.Workspace,"source.txt")
 original:=[]byte("new staged file\n")
 if err:=os.WriteFile(path,original,0600);err!=nil{t.Fatal(err)}
 remoteUnstageGitCommand(t,s.Workspace,"add","--","source.txt")
 req:=remoteUnstageRequest(t,client,s.Workspace,"")
 reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req)
 if err!=nil{t.Fatal(err)}
 if result,err:=remotetransport.OpenRemoteGitUnstageResult(client,reply,remoteDispatchTestID);err!=nil||result!="unstaged"{
  t.Fatalf("new file unstage failed: %v %q",err,result)
 }
 out:=remoteUnstageGitCommand(t,s.Workspace,"ls-files","--stage","--","source.txt")
 if len(out)!=0{t.Fatalf("new file remained staged: %q",out)}
 actual,err:=os.ReadFile(path)
 if err!=nil||!bytes.Equal(actual,original){t.Fatal("new working-tree file was removed")}
}

func TestGitUnstageHostRefusesStaleDiffAndOptOut(t *testing.T){
 t.Run("flags off",func(t *testing.T){
  s,session,client,agent:=remoteGitUnstageFixture(t)
  t.Setenv("TASKDECK_REMOTE_GIT_UNSTAGE","")
  path:=filepath.Join(s.Workspace,"source.txt")
  if err:=os.WriteFile(path,[]byte("base\n"),0600);err!=nil{t.Fatal(err)}
  remoteUnstageSetupBase(t,s.Workspace)
  if err:=os.WriteFile(path,[]byte("staged\n"),0600);err!=nil{t.Fatal(err)}
  remoteUnstageGitCommand(t,s.Workspace,"add","--","source.txt")
  req:=remoteUnstageRequest(t,client,s.Workspace,"")
  if reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req);err==nil||len(reply)!=0{
   t.Fatal("unstage ignored disabled host flag")
  }
  got:=remoteUnstageGitCommand(t,s.Workspace,"show",":source.txt")
  if !bytes.Equal(got,[]byte("staged\n")){t.Fatal("unauthorized unstage altered index")}
 })
 t.Run("stale staged diff CAS",func(t *testing.T){
  s,session,client,agent:=remoteGitUnstageFixture(t)
  path:=filepath.Join(s.Workspace,"source.txt")
  if err:=os.WriteFile(path,[]byte("base\n"),0600);err!=nil{t.Fatal(err)}
  remoteUnstageSetupBase(t,s.Workspace)
  if err:=os.WriteFile(path,[]byte("stage 1\n"),0600);err!=nil{t.Fatal(err)}
  remoteUnstageGitCommand(t,s.Workspace,"add","--","source.txt")
  req:=remoteUnstageRequest(t,client,s.Workspace,"")
  if err:=os.WriteFile(path,[]byte("stage 2\n"),0600);err!=nil{t.Fatal(err)}
  remoteUnstageGitCommand(t,s.Workspace,"add","--","source.txt")
  if reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req);err==nil||len(reply)!=0{
   t.Fatal("stale staged diff CAS allowed unstage")
  }
  got:=remoteUnstageGitCommand(t,s.Workspace,"show",":source.txt")
  if !bytes.Equal(got,[]byte("stage 2\n")){t.Fatal("stale unstage mutated index")}
 })
}

func TestGitUnstageHostRefusesViewerAndSymlinkEscape(t *testing.T){
 t.Run("viewer denied",func(t *testing.T){
  s,session,client,agent:=remoteDispatchFixtureWithRole(t,remotetransport.RoleViewer)
  t.Setenv("TASKDECK_REMOTE_GIT_WRITE","1")
  t.Setenv("TASKDECK_REMOTE_GIT_UNSTAGE","1")
  if err:=exec.Command("git","init","-q",s.Workspace).Run();err!=nil{t.Skipf("Git unavailable: %v",err)}
  path:=filepath.Join(s.Workspace,"source.txt")
  if err:=os.WriteFile(path,[]byte("base\n"),0600);err!=nil{t.Fatal(err)}
  remoteUnstageSetupBase(t,s.Workspace)
  if err:=os.WriteFile(path,[]byte("staged\n"),0600);err!=nil{t.Fatal(err)}
  remoteUnstageGitCommand(t,s.Workspace,"add","--","source.txt")
  req:=remoteUnstageRequest(t,client,s.Workspace,"")
  if reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req);err==nil||len(reply)!=0{
   t.Fatal("viewer can unstage without GitWrite")
  }
 })
 t.Run("symlink denied",func(t *testing.T){
  s,session,client,agent:=remoteGitUnstageFixture(t)
  outside:=filepath.Join(t.TempDir(),"private")
  original:=[]byte("outside private\n")
  if err:=os.WriteFile(outside,original,0600);err!=nil{t.Fatal(err)}
  if err:=os.Symlink(outside,filepath.Join(s.Workspace,"source.txt"));err!=nil{t.Fatal(err)}
  badSHA:=strings.Repeat("a",64)
  request,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
   RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionGitUnstage,
   Path:"source.txt",ExpectedSHA256:badSHA,
  })
  if err!=nil{t.Fatal(err)}
  if reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,request);err==nil||len(reply)!=0{
   t.Fatal("symlink escaped workspace during unstage")
  }
 })
}

func TestGitUnstageIndexRecordParsersRejectConflictsAndPaths(t *testing.T){
 for _,x:=range []string{
  "100644 "+strings.Repeat("a",40)+" 2\tsource.txt\x00",
  "100644 "+strings.Repeat("a",40)+" 0\tother.txt\x00",
  "120000 "+strings.Repeat("a",40)+" 0\tsource.txt\x00",
  "100644 "+strings.Repeat("a",40)+" 0\tsource.txt\x00extra",
 }{
  if _,_,ok:=remoteOneIndexEntry([]byte(x),"source.txt");ok{
   t.Fatalf("unsafe index entry accepted: %q",x)
  }
 }
 for _,p:=range []string{"-option","../x","sub/*",".git/index",".GIT/config","src/.Git/config","a[abc]","a?b","x:y"} {
  if remoteUnstageLiteralPath(p){
   t.Fatalf("pathspec/magic unstage accepted: %q",p)
  }
 }
}
