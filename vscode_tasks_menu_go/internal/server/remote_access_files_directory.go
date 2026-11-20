package server

import (
 "context"
 "crypto/sha256"
 "io"
 "os"
 "path/filepath"
 "runtime"
 "sort"
 "strings"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// The host, not the browser, controls which workspace-root directories may
// be enumerated. A selected index must refer to a D: item actually issued
// to this very E2EE session. Never accept path strings from the transport.
func remoteFilesDirAllowed(name string)bool{
 raw:=os.Getenv("TASKDECK_REMOTE_FILES_DIR_NAMES")
 if raw==""||len(raw)>512{return false}
 names:=strings.Split(raw,",")
 if len(names)>8{return false}
 seen:=make(map[string]bool,len(names))
 allowed:=false
 for _,entry:=range names{
  if !remotetransport.ValidRemoteFilesRootItem("D:"+entry)||seen[entry]{
   return false
  }
  seen[entry]=true
  if entry==name{allowed=true}
 }
 return allowed
}

// Exactly one level, names only. Open with O_NOFOLLOW and verify the
// descriptor against both the pre- and post-read directory inode.
// Do not follow symlinks inside the folder; unreadable/racy metadata
// is a denial rather than a partially trusted listing.
func(s *Server)remoteFilesDirSnapshot(ctx context.Context,root,name string)([]string,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if s==nil||ctx==nil||ctx.Err()!=nil||s.Workspace==""||
  runtime.GOOS=="windows"||os.Getenv("TASKDECK_REMOTE_FILES_ROOT")!="1"||
  os.Getenv("TASKDECK_REMOTE_FILES_DIRS")!="1"||
  !remoteFilesDirAllowed(name)||!remotetransport.ValidRemoteFilesRootItem("D:"+name){
  return nil,denied
 }
 location:=filepath.Join(root,name)
 before,err:=os.Lstat(location)
 if err!=nil||!before.IsDir()||before.Mode()&os.ModeSymlink!=0{
  return nil,denied
 }
 handle,err:=openRemoteFileNoFollow(root,name)
 if err!=nil{return nil,denied}
 defer handle.Close()
 opened,err:=handle.Stat()
 if err!=nil||!opened.IsDir()||!os.SameFile(before,opened){
  return nil,denied
 }
 entries,err:=handle.ReadDir(129)
 if err!=nil&&err!=io.EOF{return nil,denied}
 if len(entries)>=129{return nil,denied}
 names:=make([]string,0,len(entries))
 for _,entry:=range entries{
  if ctx.Err()!=nil{return nil,denied}
  n:=entry.Name()
  if !remotetransport.ValidRemoteFilesRootItem("F:"+n){continue}
  stat,err:=os.Lstat(filepath.Join(location,n))
  if err!=nil{return nil,denied}
  kind:=""
  if stat.Mode().IsRegular(){kind="F:"}
  if stat.IsDir(){kind="D:"}
  if kind==""{continue}
  names=append(names,kind+n)
 }
 // A raced rename, deletion or directory replacement must not allow the
 // Agent to page names from a no-longer-host-authorized folder.
 after,err:=os.Lstat(location)
 if err!=nil||!after.IsDir()||!os.SameFile(before,after)||
  !os.SameFile(opened,after)||before.ModTime()!=after.ModTime(){
  return nil,denied
 }
 currentRoot,err:=filepath.EvalSymlinks(s.Workspace)
 if err!=nil{return nil,denied}
 currentRoot,err=filepath.Abs(currentRoot)
 if err!=nil||filepath.Clean(currentRoot)!=filepath.Clean(root)||
  ctx.Err()!=nil{return nil,denied}
 sort.Slice(names,func(i,j int)bool{return names[i][2:]<names[j][2:]})
 return names,nil
}

func(s *Server)readRemoteDirFilesPage(ctx context.Context,
 session *remoteTaskDeckSession,first bool,index int,
)(remotetransport.RemoteFilesRoot,error){
 denied:=remotetransport.ErrRemoteActionDenied
 var empty remotetransport.RemoteFilesRoot
 if s==nil||ctx==nil||ctx.Err()!=nil||session==nil||
  session.closed.Load()||session.channel==nil||
  !session.channel.Established()||session.portalGrant==nil||
  !session.portalGrant.ActiveForSession(session.channel)||
  (first&&!remotetransport.ValidRemoteFilesDirSelection(index))||
  (!first&&index!=0){return empty,denied}
 session.dirFilesMu.Lock()
 defer session.dirFilesMu.Unlock()
 if !first&&!session.dirFilesReady{return empty,denied}
 session.rootFilesMu.Lock()
 rootNames,root,err:=s.remoteFilesRootSnapshot(ctx)
 if err!=nil{
  session.rootFilesMu.Unlock()
  session.dirFilesReady=false
  return empty,denied
 }
 rootDigest:=sha256.Sum256([]byte(root+"\x00"+strings.Join(rootNames,"\x00")))
 if !first{index=session.dirFilesIndex}
 if index<1||index>len(rootNames)||index>session.rootFilesCursor||
  session.rootFilesDigest!=rootDigest||
  rootNames[index-1][:2]!="D:"{
  session.rootFilesMu.Unlock()
  session.dirFilesReady=false
  return empty,denied
 }
 name:=rootNames[index-1][2:]
 session.rootFilesMu.Unlock()
 if !remoteFilesDirAllowed(name){
  session.dirFilesReady=false;return empty,denied
 }
 if !first&&(session.dirFilesName!=name||
  session.dirFilesRootDigest!=rootDigest){
  session.dirFilesReady=false;return empty,denied
 }
 names,err:=s.remoteFilesDirSnapshot(ctx,root,name)
 if err!=nil||ctx.Err()!=nil||session.closed.Load(){
  session.dirFilesReady=false;return empty,denied
 }
 digest:=sha256.Sum256([]byte(root+"\x00"+name+"\x00"+
  string(rootDigest[:])+"\x00"+strings.Join(names,"\x00")))
 offset:=0
 if !first{
  offset=session.dirFilesCursor
  if session.dirFilesDigest!=digest||offset<1||offset>=len(names){
   session.dirFilesReady=false;return empty,denied
  }
 }
 result:=remoteFilesRootSlice(names,offset)
 if !remotetransport.ValidRemoteFilesRoot(result)||ctx.Err()!=nil||
  session.closed.Load(){session.dirFilesReady=false;return empty,denied}
 session.dirFilesReady=result.More
 session.dirFilesIndex=index
 session.dirFilesName=name
 session.dirFilesCursor=offset+len(result.Items)
 session.dirFilesDigest=digest
 session.dirFilesRootDigest=rootDigest
 return result,nil
}
