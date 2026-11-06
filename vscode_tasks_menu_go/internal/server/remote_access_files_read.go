package server

import (
 "bytes"
 "context"
 "crypto/sha256"
 "io"
 "os"
 "path/filepath"
 "runtime"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// The client NEVER supplies a file path. The trusted host must opt into
// read-only, text-only disclosure of exactly one named workspace-root file.
// This does not permit arbitrary file reads, recursion, downloads or editing.
func (s *Server)remoteFileReadSnapshot(ctx context.Context)(string,string,string,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if s==nil||ctx==nil||ctx.Err()!=nil||s.Workspace==""||
  runtime.GOOS=="windows"||
  os.Getenv("TASKDECK_REMOTE_FILES_ROOT")!="1"||
  os.Getenv("TASKDECK_REMOTE_FILES_READ")!="1"{
  return "","","",denied
 }
 name:=os.Getenv("TASKDECK_REMOTE_FILES_READ_NAME")
 if !remotetransport.ValidRemoteFilesRootItem("F:"+name){
  return "","","",denied
 }
 names,root,err:=s.remoteFilesRootSnapshot(ctx)
 if err!=nil{return "","","",denied}
 found:=false
 for _,value:=range names{if value=="F:"+name{found=true;break}}
 if !found{return "","","",denied}
 path:=filepath.Join(root,name)
 before,err:=os.Lstat(path)
 if err!=nil||!before.Mode().IsRegular()||
  before.Mode()&os.ModeSymlink!=0||
  before.Size()<0||before.Size()>remotetransport.MaxRemoteFilesReadSourceBytes{
  return "","","",denied
 }
 // Platform-specific O_NOFOLLOW: Linux pins the directory using openat,
 // macOS denies final-component symlinks. The selected file must still be
 // the SAME regular inode in the host workspace after reading.
 handle,err:=openRemoteFileNoFollow(root,name)
 if err!=nil{return "","","",denied}
 defer handle.Close()
 opened,err:=handle.Stat()
 if err!=nil||!opened.Mode().IsRegular()||!os.SameFile(before,opened)||
  opened.Size()!=before.Size(){return "","","",denied}
 // Read a whole bounded file rather than streaming from a process pipe.
 // The second read detects changes during the first. A later page
 // independently repeats the entire snapshot and compares its hash.
 limit:=int64(remotetransport.MaxRemoteFilesReadSourceBytes+1)
 data,err:=io.ReadAll(io.LimitReader(handle,limit))
 if err!=nil||len(data)>remotetransport.MaxRemoteFilesReadSourceBytes||
  int64(len(data))!=before.Size(){return "","","",denied}
 if _,err:=handle.Seek(0,io.SeekStart);err!=nil{return "","","",denied}
 again,err:=io.ReadAll(io.LimitReader(handle,limit))
 if err!=nil||!bytes.Equal(data,again){return "","","",denied}
 after,err:=os.Lstat(path)
 if err!=nil||!after.Mode().IsRegular()||
  !os.SameFile(before,after)||!os.SameFile(opened,after)||
  before.ModTime()!=after.ModTime()||before.Size()!=after.Size()||
  !remotetransport.ValidRemoteFilesReadText(string(data))||
  ctx.Err()!=nil{return "","","",denied}
 // Re-check that root is still the same configured directory after read.
 checkRoot,err:=filepath.EvalSymlinks(s.Workspace)
 if err!=nil{return "","","",denied}
 checkRoot,err=filepath.Abs(checkRoot)
 if err!=nil||filepath.Clean(checkRoot)!=filepath.Clean(root){
  return "","","",denied
 }
 return name,string(data),root,nil
}

func (s *Server)readRemoteFileTextPage(
 ctx context.Context,session *remoteTaskDeckSession,first bool,id string,
)(remotetransport.RemoteFilesReadPage,error){
 denied:=remotetransport.ErrRemoteActionDenied
 var empty remotetransport.RemoteFilesReadPage
 if session==nil||session.closed.Load()||session.channel==nil||
  !session.channel.Established()||session.portalGrant==nil||
  !session.portalGrant.ActiveForSession(session.channel){
  return empty,denied
 }
 session.fileReadMu.Lock()
 defer session.fileReadMu.Unlock()
 if !first&&!session.fileReadReady{return empty,denied}
 name,whole,root,err:=s.remoteFileReadSnapshot(ctx)
 if err!=nil||ctx.Err()!=nil||session.closed.Load(){
  session.fileReadReady=false;return empty,denied
 }
 sum:=sha256.Sum256([]byte(root+"\x00"+name+"\x00"+whole))
 offset,pageNo:=0,1
 if !first{
  if session.fileReadDigest!=sum||session.fileReadName!=name||
   session.fileReadOffset>=len(whole){
   session.fileReadReady=false;return empty,denied
  }
  offset,pageNo=session.fileReadOffset,session.fileReadPage+1
 }
 page,end,err:=remotetransport.BoundRemoteFilesReadPage(id,name,whole,offset,pageNo)
 if err!=nil||ctx.Err()!=nil||session.closed.Load(){
  session.fileReadReady=false;return empty,denied
 }
 if page.More{
  session.fileReadDigest=sum
  session.fileReadName=name
  session.fileReadOffset=end
  session.fileReadPage=pageNo
  session.fileReadReady=true
 }else{
  session.fileReadReady=false
  session.fileReadDigest=[32]byte{}
  session.fileReadName=""
  session.fileReadOffset=0
  session.fileReadPage=0
 }
 return page,nil
}
