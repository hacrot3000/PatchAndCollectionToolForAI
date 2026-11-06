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

// Only an opt-in root directory listing. No PHP/browser selected path,
// recursive traversal or content transfer. The full bounded snapshot is
// re-read before each session-owned page; changed roots fail closed.
func (s *Server) remoteFilesRootSnapshot(ctx context.Context)([]string,string,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if s==nil||ctx==nil||ctx.Err()!=nil||s.Workspace==""||
  runtime.GOOS=="windows"||os.Getenv("TASKDECK_REMOTE_FILES_ROOT")!="1"{
  return nil,"",denied
 }
 root,err:=filepath.EvalSymlinks(s.Workspace)
 if err!=nil{return nil,"",denied}
 root,err=filepath.Abs(root)
 if err!=nil{return nil,"",denied}
 info,err:=os.Lstat(root)
 if err!=nil||!info.IsDir()||info.Mode()&os.ModeSymlink!=0{
  return nil,"",denied
 }
 // Pin an actual directory descriptor to the already canonical root.
 // On Linux this uses openat from the parent with O_NOFOLLOW; on macOS
 // the final component is opened O_NOFOLLOW and checked by inode.
 handle,err:=openRemoteFileNoFollow(filepath.Dir(root),filepath.Base(root))
 if err!=nil{return nil,"",denied}
 defer handle.Close()
 opened,err:=handle.Stat()
 if err!=nil||!opened.IsDir()||!os.SameFile(info,opened){
  return nil,"",denied
 }
 // Reject directories with more than 128 entries instead of reading a
 // potentially unbounded tree or disclosing a misleading partial snapshot.
 entries,err:=handle.ReadDir(129)
 if err!=nil&&err!=io.EOF{return nil,"",denied}
 if len(entries)>=129{return nil,"",denied}
 names:=make([]string,0,len(entries))
 for _,entry:=range entries{
  if ctx.Err()!=nil{return nil,"",denied}
  name:=entry.Name()
  if !remotetransport.ValidRemoteFilesRootItem("F:"+name){continue}
  fi,e:=os.Lstat(filepath.Join(root,name))
  if e!=nil{return nil,"",denied}
  kind:=""
  if fi.Mode().IsRegular(){kind="F:"}
  if fi.IsDir(){kind="D:"}
  if kind==""{continue} // Never follow a symlink or report sockets/devices.
  names=append(names,kind+name)
 }
 after,err:=os.Lstat(root)
 if err!=nil||!after.IsDir()||!os.SameFile(info,after)||
  !os.SameFile(opened,after)||info.ModTime()!=after.ModTime(){
  return nil,"",denied
 }
 // A replaced root path must not silently become another source during
 // a read or while the client is paging the E2EE snapshot.
 latest,err:=filepath.EvalSymlinks(s.Workspace)
 if err!=nil{return nil,"",denied}
 latest,err=filepath.Abs(latest)
 if err!=nil||filepath.Clean(latest)!=filepath.Clean(root)||ctx.Err()!=nil{
  return nil,"",denied
 }
 sort.Slice(names,func(i,j int)bool{return names[i][2:]<names[j][2:]})
 return names,root,nil
}

func remoteFilesRootSlice(names []string,offset int)remotetransport.RemoteFilesRoot{
 end:=offset+remotetransport.MaxRemoteFilesRootItems
 if end>len(names){end=len(names)}
 return remotetransport.RemoteFilesRoot{
  Items:append([]string{},names[offset:end]...),More:end<len(names),
 }
}

// The original direct helper remains a deterministic bounded first page,
// without creating any hidden remote session or transferring a cursor.
func(s *Server)readRemoteFilesRoot(ctx context.Context)(remotetransport.RemoteFilesRoot,error){
 names,_,err:=s.remoteFilesRootSnapshot(ctx)
 if err!=nil{return remotetransport.RemoteFilesRoot{},err}
 return remoteFilesRootSlice(names,0),nil
}

// Pagination is private to one E2EE session, not an offset or token that PHP
// can replay. Switching workspace, removing/adding/renaming root entries,
// or changing file type invalidates every outstanding next-page request.
func(s *Server)readRemoteFilesRootPage(
 ctx context.Context,session *remoteTaskDeckSession,first bool,
)(remotetransport.RemoteFilesRoot,error){
 denied:=remotetransport.ErrRemoteActionDenied
 var empty remotetransport.RemoteFilesRoot
 if session==nil||session.closed.Load()||session.channel==nil||
  !session.channel.Established(){return empty,denied}
 session.rootFilesMu.Lock()
 defer session.rootFilesMu.Unlock()
 names,root,err:=s.remoteFilesRootSnapshot(ctx)
 if err!=nil{session.rootFilesReady=false;return empty,denied}
 sum:=sha256.Sum256([]byte(root+"\x00"+strings.Join(names,"\x00")))
 offset:=0
 if !first{
  if !session.rootFilesReady||session.rootFilesDigest!=sum||
   session.rootFilesCursor<1||session.rootFilesCursor>=len(names){
   session.rootFilesReady=false;return empty,denied
  }
  offset=session.rootFilesCursor
 }
 result:=remoteFilesRootSlice(names,offset)
 if !remotetransport.ValidRemoteFilesRoot(result)||ctx.Err()!=nil||
  session.closed.Load(){session.rootFilesReady=false;return empty,denied}
 session.rootFilesReady=result.More
 session.rootFilesCursor=offset+len(result.Items)
 session.rootFilesDigest=sum
 return result,nil
}
