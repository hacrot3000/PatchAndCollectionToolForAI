package server

import (
 "context"
 "os/exec"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
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
