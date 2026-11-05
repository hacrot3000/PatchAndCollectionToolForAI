package server

import (
 "bytes"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "io"
 "net/url"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "time"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// This deliberately narrow network capability is limited to a public,
// credentialless, HOST-PINNED HTTPS URL matching the repository's origin.
// No remote input (PHP, Relay, browser, QR code) supplies any URL/refspec,
// auth header, shell command or Git argv. Other transports and private
// repositories remain unsupported, rather than risking credential leakage.
func remoteGitTrustedFetchURL(raw string)(string,bool){
 if raw==""||len(raw)>240||strings.ContainsAny(raw," 	\r\n\\%"){
  return "",false
 }
 u,err:=url.Parse(raw)
 if err!=nil||u==nil||u.Scheme!="https"||u.Opaque!=""||
  u.User!=nil||u.RawQuery!=""||u.ForceQuery||u.Fragment!=""||
  u.RawFragment!=""||u.RawPath!=""||u.Port()!=""{
  return "",false
 }
 host:=u.Hostname()
 if u.Host!=host||(host!="github.com"&&host!="gitlab.com"&&host!="bitbucket.org"){
  return "",false
 }
 if !strings.HasPrefix(u.Path,"/")||strings.HasSuffix(u.Path,"/"){return "",false}
 parts:=strings.Split(strings.TrimPrefix(u.Path,"/"),"/")
 if len(parts)<2||len(parts)>6{return "",false}
 for _,segment:=range parts{
  if !validRemoteGitName(segment)||segment=="HEAD"||
   strings.HasSuffix(segment,".lock"){return "",false}
 }
 if u.String()!=raw{return "",false}
 return host,true
}

// Reject all repo-local keys that could alter Git transport, redirect a
// repository URL, call a helper, load arbitrary config or change TLS trust.
// Deliberately conservative: custom Git configuration uses local Git, not
// an E2EE Remote Fetch. The local config is read WITHOUT following includes.
func remoteGitHTTPSFetchConfigKeysSafe(ctx context.Context,root,git,branchName string)bool{
 if ctx==nil||ctx.Err()!=nil||root==""||git==""||branchName==""{
  return false
 }
 cmd:=exec.CommandContext(ctx,git,"-C",root,
  "config","--local","--name-only","--list")
 cmd.Env=isolatedRemoteGitEnvironment()
 cmd.Stderr=io.Discard
 var out cappedRemoteGitDiffOutput
 cmd.Stdout=&out
 if cmd.Run()!=nil||ctx.Err()!=nil{return false}
 keys:=strings.Split(strings.TrimSuffix(string(out.Bytes()),"\n"),"\n")
 if len(keys)<4||len(keys)>60{return false}
 allowed:=map[string]bool{
  "core.repositoryformatversion":true,
  "core.filemode":true,
  "core.bare":true,
  "core.logallrefupdates":true,
  "core.ignorecase":true,
  "core.precomposeunicode":true,
  "core.quotepath":true,
  "core.autocrlf":true,
  "core.symlinks":true,
  "core.untrackedcache":true,
  "remote.origin.url":true,
  "remote.origin.fetch":true,
  "branch."+branchName+".remote":true,
  "branch."+branchName+".merge":true,
  "user.name":true,
  "user.email":true,
  "pull.rebase":true,
  "pull.ff":true,
  "fetch.prune":true,
  "init.defaultbranch":true,
 }
 for _,name:=range keys{
  if !allowed[name]{return false}
 }
 return true
}

func remoteGitHTTPSFetchConfigBytes(root string)([]byte,error){
 denied:=remotetransport.ErrRemoteActionDenied
 path:=filepath.Join(root,".git","config")
 info,err:=os.Lstat(path)
 if err!=nil||!info.Mode().IsRegular()||info.Mode()&os.ModeSymlink!=0||
  info.Size()<1||info.Size()>1<<20{return nil,denied}
 f,err:=os.Open(path)
 if err!=nil{return nil,denied}
 defer f.Close()
 data,err:=io.ReadAll(io.LimitReader(f,1<<20+1))
 if err!=nil||len(data)<1||len(data)>1<<20{return nil,denied}
 return data,nil
}

func remoteGitHTTPSFetchConfigOriginSafe(ctx context.Context,root,git,expected string)bool{
 if ctx==nil||ctx.Err()!=nil||git==""||expected==""{return false}
 cmd:=exec.CommandContext(ctx,git,"-C",root,
  "config","--local","--get-all","remote.origin.url")
 cmd.Env=isolatedRemoteGitEnvironment()
 cmd.Stderr=io.Discard
 var out cappedRemoteGitDiffOutput
 cmd.Stdout=&out
 if cmd.Run()!=nil||ctx.Err()!=nil{return false}
 return string(out.Bytes())==expected+"\n"
}

// Git fetch can append to the remote-tracking reflog and replace FETCH_HEAD.
// Each writable ancestor must be a real in-workspace directory, not a link.
func remoteGitHTTPSFetchMetadataSafe(root,branch,upstream string)bool{
 if !remoteGitPullFilesMetadataSafe(root,branch,upstream){return false}
 prefix:="refs/remotes/origin/"
 if !strings.HasPrefix(upstream,prefix){return false}
 name:=strings.TrimPrefix(upstream,prefix)
 if !remoteGitPullBranchSafe(name){return false}
 parent:=filepath.Join(root,".git","logs","refs","remotes")
 parts:=strings.Split("origin/"+name,"/")
 for _,piece:=range parts[:len(parts)-1]{
  parent=filepath.Join(parent,piece)
  info,e:=os.Lstat(parent)
  if e!=nil||!info.IsDir()||info.Mode()&os.ModeSymlink!=0{return false}
 }
 leaf:=filepath.Join(parent,parts[len(parts)-1])
 if info,e:=os.Lstat(leaf);e==nil{
  if !info.Mode().IsRegular()||info.Mode()&os.ModeSymlink!=0{return false}
 }else if !os.IsNotExist(e){return false}
 for _,candidate:=range []string{"FETCH_HEAD","shallow","FETCH_HEAD.lock"}{
  leaf=filepath.Join(root,".git",candidate)
  if info,e:=os.Lstat(leaf);e==nil{
   if !info.Mode().IsRegular()||info.Mode()&os.ModeSymlink!=0{return false}
  }else if !os.IsNotExist(e){return false}
 }
 return true
}

func remoteGitHTTPSFetchReview(ctx context.Context,root string)([]byte,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if ctx==nil||ctx.Err()!=nil||!remoteGitPullFilesWorktreeConfigSafe(ctx,root){
  return nil,denied
 }
 pinnedURL:=os.Getenv("TASKDECK_REMOTE_GIT_FETCH_URL")
 host,ok:=remoteGitTrustedFetchURL(pinnedURL)
 if !ok{return nil,denied}
 preview,err:=remoteGitPullReview(ctx,root)
 if err!=nil||!remotetransport.ValidRemoteGitPullReviewReport(string(preview)){
  return nil,denied
 }
 rows:=strings.Split(string(preview),"\n")
 if len(rows)!=9{return nil,denied}
 branch:=strings.TrimPrefix(rows[1],"BRANCH ")
 head:=strings.TrimPrefix(rows[2],"HEAD ")
 tracking:=strings.TrimPrefix(rows[3],"UPSTREAM ")
 target:=strings.TrimPrefix(rows[4],"UPSTREAM_HEAD ")
 name:=strings.TrimPrefix(branch,"refs/heads/")
 if !strings.HasPrefix(branch,"refs/heads/")||
  tracking!="refs/remotes/origin/"+name||
  !remoteGitPullBranchSafe(name)||
  !remoteGitHTTPSFetchMetadataSafe(root,branch,tracking){return nil,denied}
 git,err:=remoteTrustedGitExecutable(root)
 if err!=nil||remoteGitReadOnlyConfigSafe(ctx,root,git)!=nil||
  !remoteGitHTTPSFetchConfigKeysSafe(ctx,root,git,name)||
  !remoteGitHTTPSFetchConfigOriginSafe(ctx,root,git,pinnedURL){
  return nil,denied
 }
 config,err:=remoteGitHTTPSFetchConfigBytes(root)
 if err!=nil{return nil,denied}
 urlHash:=sha256.Sum256([]byte(pinnedURL))
 configHash:=sha256.Sum256(config)
 report:="GIT_FETCH_HTTPS_REVIEW_V1\nBRANCH "+branch+"\nHEAD "+head+
  "\nUPSTREAM "+tracking+"\nUPSTREAM_HEAD "+target+
  "\nHTTPS_HOST "+host+"\nURL_SHA256 "+hex.EncodeToString(urlHash[:])+
  "\nCONFIG_SHA256 "+hex.EncodeToString(configHash[:])+
  "\nWORKTREE clean\nNETWORK HTTPS-approval-required\n"
 if !remotetransport.ValidRemoteGitFetchReviewReport(report){return nil,denied}
 after,err:=remoteGitPullReview(ctx,root)
 if err!=nil||!bytes.Equal(preview,after){return nil,denied}
 later,e:=remoteGitHTTPSFetchConfigBytes(root)
 if e!=nil||!bytes.Equal(config,later){return nil,denied}
 return []byte(report),nil
}

func remoteGitHTTPSFetchApply(ctx context.Context,root,approvedDigest string)(string,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if ctx==nil||ctx.Err()!=nil||!validRemoteGitSwitchDigest(approvedDigest){
  return "",denied
 }
 review,err:=remoteGitHTTPSFetchReview(ctx,root)
 if err!=nil{return "",denied}
 sum:=sha256.Sum256(review)
 if hex.EncodeToString(sum[:])!=approvedDigest{return "",denied}
 fields:=strings.Split(string(review),"\n")
 if len(fields)!=11{return "",denied}
 branch:=strings.TrimPrefix(fields[1],"BRANCH ")
 oldHead:=strings.TrimPrefix(fields[2],"HEAD ")
 upstream:=strings.TrimPrefix(fields[3],"UPSTREAM ")
 oldUpstream:=strings.TrimPrefix(fields[4],"UPSTREAM_HEAD ")
 sourceBranch:=strings.TrimPrefix(branch,"refs/heads/")
 hostURL:=os.Getenv("TASKDECK_REMOTE_GIT_FETCH_URL")
 if _,ok:=remoteGitTrustedFetchURL(hostURL);!ok{return "",denied}
 git,err:=remoteTrustedGitExecutable(root)
 if err!=nil||!remoteGitHTTPSFetchMetadataSafe(root,branch,upstream){
  return "",denied
 }
 index,err:=remoteGitPullFilesIndex(ctx,filepath.Join(root,".git","index"))
 if err!=nil{return "",denied}
 fresh,err:=remoteGitHTTPSFetchReview(ctx,root)
 if err!=nil||!bytes.Equal(fresh,review)||ctx.Err()!=nil{
  return "",denied
 }
 // The host-approved URL is passed literally, rather than the Git remote
 // name. This bypasses remote.origin.url and user-supplied fetch refspecs.
 // Reject arbitrary protocol rewriting, helpers, redirects, credentials and
 // local config overrides before the network transport can start.
 fetchCtx,cancel:=context.WithTimeout(ctx,45*time.Second)
 defer cancel()
 cmd:=exec.CommandContext(fetchCtx,git,
  "-c","core.hooksPath="+os.DevNull,
  "-c","protocol.allow=never","-c","protocol.https.allow=always",
  "-c","http.followRedirects=false",
  "-c","http.sslVerify=true",
  "-c","http.proxy=",
  "-c","credential.helper=",
  "-c","gc.auto=0","-c","maintenance.auto=false",
  "-c","fetch.writeCommitGraph=false",
  "-C",root,"fetch","--quiet","--no-tags",
  "--no-write-fetch-head","--no-recurse-submodules",
  "--no-auto-gc","--atomic","--refmap=",
  hostURL,"+refs/heads/"+sourceBranch+":"+upstream)
 cmd.Env=append(isolatedRemoteGitEnvironment(),
  "GIT_NO_REPLACE_OBJECTS=1",
  "GIT_ASKPASS=/bin/false",
  "GIT_DIR="+filepath.Join(root,".git"),
  "GIT_WORK_TREE="+root)
 cmd.Stdin=nil
 cmd.Stdout=io.Discard
 cmd.Stderr=io.Discard
 if cmd.Run()!=nil||fetchCtx.Err()!=nil{return "",denied}

 // A successful fetch is not equivalent to a safe fast-forward: the
 // refreshed tracking branch may have diverged. Never apply it here.
 currentBranch,currentHead,_,err:=remoteGitBranchRead(ctx,root)
 if err!=nil||currentBranch!=branch||currentHead!=oldHead||
  !remoteGitHTTPSFetchMetadataSafe(root,branch,upstream){return "",denied}
 currentConfig,err:=remoteGitHTTPSFetchConfigBytes(root)
 configHash:=sha256.Sum256(currentConfig)
 if err!=nil||hex.EncodeToString(configHash[:])!=
  strings.TrimPrefix(fields[7],"CONFIG_SHA256 "){return "",denied}
 currentIndex,err:=remoteGitPullFilesIndex(ctx,filepath.Join(root,".git","index"))
 if err!=nil||!bytes.Equal(currentIndex,index){return "",denied}
 // Verify the full tracking commit OID by a trusted local Git read.
 rev:=exec.CommandContext(ctx,git,"-C",root,
  "rev-parse","--verify",upstream+"^{commit}")
 rev.Env=append(isolatedRemoteGitEnvironment(),"GIT_NO_REPLACE_OBJECTS=1")
 rev.Stderr=io.Discard
 var out cappedRemoteGitDiffOutput
 rev.Stdout=&out
 if rev.Run()!=nil||ctx.Err()!=nil{return "",denied}
 newUpstream:=strings.TrimSpace(string(out.Bytes()))
 if !remoteValidGitOIDForCommitReview(newUpstream)||
  len(newUpstream)!=len(oldUpstream){return "",denied}
 // Fetch-only success: HEAD and worktree remain unchanged. A different
 // read-only FF review/explicit Apply is required to change local files.
 return newUpstream,nil
}
