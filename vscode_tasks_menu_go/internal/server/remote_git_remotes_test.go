package server

import (
 "context"
 "os/exec"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
 "bletonfc/vscode_tasks_menu/internal/identity"
)

func initRemoteGitTestRepo(t *testing.T)string{
 t.Helper()
 root:=t.TempDir()
 if output,err:=exec.Command("git","init","-q",root).CombinedOutput();err!=nil{
  t.Skipf("git unavailable: %v %s",err,output)
 }
 return root
}
func addNamedRemote(t *testing.T,root,name,url string){
 t.Helper()
 if out,err:=exec.Command("git","-C",root,"remote","add",name,url).CombinedOutput();err!=nil{
  t.Fatalf("git remote add %s: %v %s",name,err,out)
 }
}
func TestRemoteGitMetadataNamesOnlyNeverLeaksCredentialURLs(t *testing.T){
 root:=initRemoteGitTestRepo(t)
 result,err:=remoteGitMetadata(context.Background(),root,remotetransport.RemoteActionGitRemotes)
 if err!=nil||string(result)!="GIT_REMOTES_V1\n"{
  t.Fatalf("no remotes: %q %v",result,err)
 }
 token:="super-secret-remote-token-DO-NOT-EXPOSE"
 addNamedRemote(t,root,"origin","https://git-user:"+token+"@example.invalid/project.git")
 addNamedRemote(t,root,"upstream","ssh://git@example.invalid/other.git")
 result,err=remoteGitMetadata(context.Background(),root,remotetransport.RemoteActionGitRemotes)
 if err!=nil{t.Fatal(err)}
 if string(result)!="GIT_REMOTES_V1\norigin\nupstream\n"{
  t.Fatalf("unexpected safe names-only list %q",result)
 }
 for _,secret:=range []string{token,"git-user","example.invalid","https://","ssh://"}{
  if strings.Contains(string(result),secret){t.Fatalf("credential-bearing remote URL disclosed: %q",secret)}
 }
 // Listing names does not change ref or remote URL.
 url,err:=exec.Command("git","-C",root,"remote","get-url","origin").Output()
 if err!=nil||!strings.Contains(string(url),token){t.Fatalf("metadata read changed remote config: %v",err)}
}
func TestRemoteGitNamesRejectDangerousOrAmbiguousNames(t *testing.T){
 valid:=[]string{"origin","u","Upstream-1","backup_1","origin.foo"}
 invalid:=[]string{"","-bad",".hidden","a-","a.","a..b","a/b","a:b","a b","origin\nhttps://secret","ü","a\tb",strings.Repeat("a",65)}
 for _,v:=range valid{
  if !validRemoteGitName(v){t.Fatalf("legitimate safe name rejected: %q",v)}
 }
 for _,v:=range invalid{
  if validRemoteGitName(v){t.Fatalf("unsafe name accepted: %q",v)}
 }
}
func TestRemoteGitNamesFailClosedIfMoreThan16(t *testing.T){
 root:=initRemoteGitTestRepo(t)
 for i:=0;i<17;i++{
  // No network is contacted when adding a remote to local config.
  name:="r"+string(rune('a'+i))
  addNamedRemote(t,root,name,"https://example.invalid/nonexistent.git")
 }
 if output,err:=remoteGitMetadata(context.Background(),root,remotetransport.RemoteActionGitRemotes);
  err==nil||len(output)!=0{
  t.Fatalf("17 remotes should be refused as an indivisible list: %q",output)
 }
}


func TestGitRemoteNamesE2EERequiresLocalPasswordPrincipalAndAudit(t *testing.T){
 s,session,client,agent:=remoteDispatchFixture(t)
 if out,err:=exec.Command("git","init","-q",s.Workspace).CombinedOutput();err!=nil{
  t.Skipf("git unavailable: %v %s",err,out)
 }
 token:="private-token-never-relayed"
 addNamedRemote(t,s.Workspace,"origin","https://user:"+token+"@example.invalid/project.git")
 request,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionGitRemotes,
 })
 if err!=nil{t.Fatal(err)}
 if response,err:=s.executeRemoteE2EEPing(context.Background(),nil,agent,request);
  err==nil||len(response)>0{
  t.Fatal("PHP-only principal accessed Git remote metadata")
 }
 reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,request)
 if err!=nil{t.Fatal(err)}
 output,err:=remotetransport.OpenRemoteGitRemotesResult(client,reply,remoteDispatchTestID)
 if err!=nil||output!="GIT_REMOTES_V1\norigin\n"{
  t.Fatalf("invalid authenticated name-only metadata: %q %v",output,err)
 }
 if strings.Contains(output,token)||strings.Contains(output,"https://"){
  t.Fatal("remote URL/token disclosed via authenticated reply")
 }
 project,err:=s.Identity.ProjectByKey(context.Background(),session.projectKey)
 if err!=nil{t.Fatal(err)}
 audit,err:=s.Identity.ListAudit(context.Background(),identity.AuditQuery{
  ProjectID:project.ID,Action:"remote.action.git_remotes",Limit:5,
 })
 if err!=nil||len(audit)==0||audit[0].UserID==nil||*audit[0].UserID!=session.userID{
  t.Fatalf("missing TaskDeck-local E2EE remote-list audit: %v",err)
 }
}
