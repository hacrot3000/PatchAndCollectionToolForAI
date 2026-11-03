package server

import (
 "context"
 "io"
 "os/exec"
 "strconv"
 "strings"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// remoteGitCompareTracking compares immutable local commit IDs. It never
// connects to a remote or changes Git refs, index or worktree. HEAD and a
// vetted cached refs/remotes/* commit are peeled separately before rev-list,
// so concurrent ref movement cannot silently change the compared commits.
func remoteGitCompareTracking(ctx context.Context,root,name string)([]byte,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if ctx==nil||ctx.Err()!=nil||root==""||
  !remotetransport.ValidRemoteTrackingRefName(name){return nil,denied}
 git,err:=remoteTrustedGitExecutable(root)
 if err!=nil{return nil,denied}
 if err:=remoteGitReadOnlyConfigSafe(ctx,root,git);err!=nil{return nil,denied}
 head,err:=remoteGitComparisonCommit(ctx,root,git,"HEAD^{commit}")
 if err!=nil{return nil,denied}
 tracking,err:=remoteGitComparisonCommit(ctx,root,git,
  "refs/remotes/"+name+"^{commit}")
 if err!=nil{return nil,denied}
 // Only complete already-resolved OIDs are used as arguments. No remote
 // URL, helper, shell, pager, hook, user argv, fetch, pull or push is run.
 raw,err:=remoteGitComparisonOutput(ctx,root,git,
  "rev-list","--left-right","--count",head+"..."+tracking)
 if err!=nil{return nil,denied}
 counts:=strings.Fields(raw)
 if len(counts)!=2{return nil,denied}
 for _,n:=range counts{
  if !validRemoteCompareCount(n){return nil,denied}
 }
 response:="GIT_COMPARE_V1\nREF "+name+"\nHEAD "+head+
  "\nTRACKING "+tracking+"\nAHEAD "+counts[0]+"\nBEHIND "+counts[1]+"\n"
 if !remotetransport.ValidRemoteGitCompareTrackingReport(response){return nil,denied}
 return []byte(response),nil
}

func validRemoteCompareCount(v string)bool{
 if len(v)<1||len(v)>10||(len(v)>1&&v[0]=='0'){return false}
 for _,c:=range v{if c<'0'||c>'9'{return false}}
 n,err:=strconv.ParseUint(v,10,32)
 return err==nil&&n<=1000000000
}

func remoteGitComparisonCommit(ctx context.Context,root,git,ref string)(string,error){
 value,err:=remoteGitComparisonOutput(ctx,root,git,
  "rev-parse","--verify","--end-of-options",ref)
 if err!=nil{return "",err}
 if !validRemoteTrackingOID(value){return "",remotetransport.ErrRemoteActionDenied}
 return value,nil
}

func remoteGitComparisonOutput(ctx context.Context,root,git string,args ...string)(string,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if ctx==nil||ctx.Err()!=nil{return "",denied}
 command:=exec.CommandContext(ctx,git,append([]string{
  "-c","core.fsmonitor=false","-C",root,"--no-pager",
 },args...)...)
 command.Env=append(isolatedRemoteGitEnvironment(),"GIT_NO_REPLACE_OBJECTS=1")
 command.Stderr=io.Discard
 var stdout cappedRemoteGitDiffOutput
 command.Stdout=&stdout
 if err:=command.Run();err!=nil||ctx.Err()!=nil{return "",denied}
 raw:=stdout.Bytes()
 if len(raw)==0||len(raw)>256||raw[len(raw)-1]!='\n'{return "",denied}
 value:=strings.TrimSuffix(string(raw),"\n")
 if strings.ContainsAny(value,"\n\r\x00"){return "",denied}
 return value,nil
}
