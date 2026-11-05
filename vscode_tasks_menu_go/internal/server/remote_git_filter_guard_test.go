package server

import (
 "context"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// Git status and even Git diff --no-ext-diff run clean filters from
// .gitattributes. A remotely authorized read MUST never execute this code.
func TestRemoteReadOnlyGitRefusesCleanFilterExecution(t *testing.T){
 for _,op:=range []string{
  remotetransport.RemoteActionGitStatus,
  remotetransport.RemoteActionGitStatusFull,
  remotetransport.RemoteActionGitDiff,
  remotetransport.RemoteActionGitStagedDiff,
  remotetransport.RemoteActionGitHeadDiff,
  remotetransport.RemoteActionGitLog,
  remotetransport.RemoteActionGitBranches,
  remotetransport.RemoteActionGitRemotes,
  remotetransport.RemoteActionGitTrackingRefs,
 }{
  t.Run(op,func(t *testing.T){
   s,session,client,agent:=remoteDispatchFixture(t)
   if err:=exec.Command("git","init","-q",s.Workspace).Run();err!=nil{t.Skipf("git unavailable: %v",err)}
   path:=filepath.Join(s.Workspace,"source.txt")
   if err:=os.WriteFile(path,[]byte("original\n"),0600);err!=nil{t.Fatal(err)}
   for _,args:=range [][]string{
    {"-C",s.Workspace,"add","--","source.txt"},
    {"-C",s.Workspace,"-c","user.name=TaskDeck CI","-c",
     "user.email=ci@example.invalid","commit","-qm","baseline"},
   }{
    if out,err:=exec.Command("git",args...).CombinedOutput();err!=nil{
     t.Fatalf("Git fixture %v: %v %s",args,err,out)
    }
   }
   if err:=os.WriteFile(filepath.Join(s.Workspace,".gitattributes"),
    []byte("*.txt filter=evil\n"),0600);err!=nil{t.Fatal(err)}
   marker:=filepath.Join(s.Workspace,"GIT_FILTER_PWNED")
   cmd:="sh -c 'touch "+marker+"; cat'"
   if output,err:=exec.Command("git","-C",s.Workspace,"config",
    "filter.evil.clean",cmd).CombinedOutput();err!=nil{
    t.Fatalf("configure evil filter: %s %v",output,err)
   }
   if err:=os.WriteFile(path,[]byte("changed\n"),0600);err!=nil{t.Fatal(err)}
   action:=remotetransport.RemoteAction{RequestID:remoteDispatchTestID,Operation:op}
   if strings.Contains(op,"diff"){action.Path="source.txt"}
   frame,err:=remotetransport.SealRemoteAction(client,action)
   if err!=nil{t.Fatal(err)}
   if response,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,frame);
    err==nil||len(response)!=0{
    t.Fatalf("remote read-only Git ran despite untrusted clean filter: %s",op)
   }
   if _,err:=os.Stat(marker);!os.IsNotExist(err){
    t.Fatalf("remote Git read executed repository clean filter: %s %v",op,err)
   }
  })
 }
}

func TestRemoteReadOnlyGitRejectsIncludedExecutableFilterConfig(t *testing.T){
 root:=t.TempDir()
 if err:=exec.Command("git","init","-q",root).Run();err!=nil{t.Skipf("git unavailable: %v",err)}
 configPath:=filepath.Join(t.TempDir(),"included-filter.ini")
 if err:=os.WriteFile(configPath,[]byte("[filter \"danger\"]\n clean = sh -c true\n"),0600);err!=nil{t.Fatal(err)}
 if result,err:=exec.Command("git","-C",root,"config","include.path",configPath).CombinedOutput();err!=nil{
  t.Fatalf("fixture Git include: %v %s",err,result)
 }
 executable,err:=remoteTrustedGitExecutable(root)
 if err!=nil{t.Skipf("no approved Git binary: %v",err)}
 if err:=remoteGitReadOnlyConfigSafe(context.Background(),root,executable);err==nil{
  t.Fatal("included clean filter evaded remote read-only safety guard")
 }
}
