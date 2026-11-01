package server

import (
 "bytes"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "io"
 "os"
 "os/exec"
 "path/filepath"
 "strings"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// The only accepted targets are literal source filenames, not Git pathspecs
// (Git interprets ?, *, [] as patterns EVEN following --).
func remoteUnstageLiteralPath(p string)bool{
 return p!=""&&len(p)<=120&&!strings.HasPrefix(p,"-")&&
  !strings.ContainsAny(p,"*?[]\\\r\n\x00")&&!strings.Contains(p,"..")&&
  !strings.Contains(p,":")&&!strings.HasPrefix(p,"/")
}

// This is a strict parser for one NUL-terminated Git index entry (stage 0).
// Conflicted/unmerged index entries are never silently resolved.
func remoteOneIndexEntry(data []byte,p string)(mode,oid string,ok bool){
 if len(data)==0||data[len(data)-1]!=0{return "","",false}
 records:=bytes.Split(data[:len(data)-1],[]byte{0})
 if len(records)!=1{return "","",false}
 record:=records[0]
 parts:=bytes.SplitN(record,[]byte{'\t'},2)
 if len(parts)!=2||string(parts[1])!=p{return "","",false}
 fields:=strings.Fields(string(parts[0]))
 if len(fields)!=3||fields[2]!="0"{return "","",false}
 if fields[0]!="100644"&&fields[0]!="100755"{return "","",false}
 if !remoteValidGitOID(fields[1]){return "","",false}
 return fields[0],fields[1],true
}

func remoteValidGitOID(oid string)bool{
 if len(oid)!=40&&len(oid)!=64{return false}
 for _,c:=range oid{
  if !((c>='0'&&c<='9')||(c>='a'&&c<='f')){return false}
 }
 return true
}

// remoteOneHeadBlob allows absence in HEAD for a newly staged file.
// Linked/merged tree objects, symlinks and submodules are denied.
func remoteOneHeadBlob(data []byte,p string)(mode,oid string,exists,ok bool){
 if len(data)==0{return "","",false,true}
 if data[len(data)-1]!=0{return "","",false,false}
 records:=bytes.Split(data[:len(data)-1],[]byte{0})
 if len(records)!=1{return "","",false,false}
 parts:=bytes.SplitN(records[0],[]byte{'\t'},2)
 if len(parts)!=2||string(parts[1])!=p{return "","",false,false}
 fields:=strings.Fields(string(parts[0]))
 if len(fields)!=3||fields[1]!="blob"||
  (fields[0]!="100644"&&fields[0]!="100755")||
  !remoteValidGitOID(fields[2]){return "","",false,false}
 return fields[0],fields[2],true,true
}

// Only Git plumbing commands with literal fixed arguments. Caller holds
// authenticated session.executionMu and has audited/rechecked all rights.
// Index update is lockfile-protected by Git; external changes between a final
// CAS check and update must still be considered an operational limitation.
func remoteUnstageOneGitFile(ctx context.Context,root,p,expectedDiffSHA string)error{
 denied:=remotetransport.ErrRemoteActionDenied
 if ctx==nil||ctx.Err()!=nil||!remoteUnstageLiteralPath(p)||
  len(expectedDiffSHA)!=64{return denied}
 for _,c:=range expectedDiffSHA{
  if !((c>='0'&&c<='9')||(c>='a'&&c<='f')){return denied}
 }
 gitDir:=filepath.Join(root,".git")
 info,err:=os.Lstat(gitDir)
 if err!=nil||!info.IsDir()||info.Mode()&os.ModeSymlink!=0{return denied}
 for _,child:=range []string{"objects","index"}{
  childInfo,e:=os.Lstat(filepath.Join(gitDir,child))
  if e!=nil{
   if child=="index"&&os.IsNotExist(e){continue}
   return denied
  }
  if childInfo.Mode()&os.ModeSymlink!=0{return denied}
  if child=="objects"&&!childInfo.IsDir(){return denied}
  if child=="index"&&!childInfo.Mode().IsRegular(){return denied}
 }
 executable,err:=remoteTrustedGitExecutable(root)
 if err!=nil{return denied}
 if err:=remoteGitReadOnlyConfigSafe(ctx,root,executable);err!=nil{return denied}
 common:=[]string{"-c","core.fsmonitor=false","-c","core.untrackedCache=false",
  "-c","core.hooksPath="+os.DevNull,"-c","diff.external=","-C",root}
 run:=func(args ...string)([]byte,error){
  cmd:=exec.CommandContext(ctx,executable,append(append([]string(nil),common...),args...)...)
  cmd.Env=isolatedRemoteGitEnvironment()
  cmd.Stderr=io.Discard
  var out cappedRemoteGitDiffOutput
  cmd.Stdout=&out
  if e:=cmd.Run();e!=nil||ctx.Err()!=nil{return nil,denied}
  return append([]byte(nil),out.Bytes()...),nil
 }
 top,err:=run("rev-parse","--show-toplevel")
 if err!=nil||filepath.Clean(strings.TrimSpace(string(top)))!=filepath.Clean(root){return denied}
 index,err:=run("ls-files","--stage","-z","--",p)
 if err!=nil{return denied}
 _,stagedOID,ok:=remoteOneIndexEntry(index,p)
 if !ok{return denied}
 diff,err:=run("--no-pager","diff","--cached","--no-ext-diff","--no-textconv",
  "--no-color","--",p)
 if err!=nil||len(diff)==0||bytes.Contains(diff,[]byte("Binary files"))||
  bytes.Contains(diff,[]byte("GIT binary patch")){return denied}
 digest:=sha256.Sum256(diff)
 if hex.EncodeToString(digest[:])!=expectedDiffSHA{return denied}
 // Git's fixed HEAD tree is the ONLY permitted restore target.
 // No arbitrary revision, Git reset/checkout or shell supplied by client.
 head,err:=run("ls-tree","-z","HEAD","--",p)
 if err!=nil{return denied}
 mode,oid,exists,ok:=remoteOneHeadBlob(head,p)
 if !ok{return denied}
 if exists&&stagedOID==oid{return denied} // no staged content change
 // Final staged diff and entry re-check immediately before mutation.
 latest,err:=run("ls-files","--stage","-z","--",p)
 if err!=nil||!bytes.Equal(latest,index){return denied}
 latestDiff,err:=run("--no-pager","diff","--cached","--no-ext-diff",
  "--no-textconv","--no-color","--",p)
 if err!=nil||!bytes.Equal(latestDiff,diff){return denied}
 if exists{
  if _,err=run("update-index","--add","--cacheinfo",mode+","+oid+","+p);err!=nil{return denied}
 }else{
  if _,err=run("update-index","--force-remove","--",p);err!=nil{return denied}
 }
 verified,err:=run("ls-files","--stage","-z","--",p)
 if err!=nil{return denied}
 if exists {
  actualMode,actualOID,ok:=remoteOneIndexEntry(verified,p)
  if !ok||actualMode!=mode||actualOID!=oid{return denied}
 }else if len(verified)!=0{return denied}
 return ctx.Err()
}
