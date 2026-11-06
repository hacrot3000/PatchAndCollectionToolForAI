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
 info,err:=os.Stat(root)
 if err!=nil||!info.IsDir(){return nil,"",denied}
 handle,err:=os.Open(root)
 if err!=nil{return nil,"",denied}
 defer handle.Close()
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
