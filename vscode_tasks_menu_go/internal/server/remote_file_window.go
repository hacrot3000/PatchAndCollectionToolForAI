package server

import (
 "context"
 "crypto/sha256"
 "encoding/hex"
 "os"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// remoteFileWindowSnapshotStable rejects files changed while the pinned
// descriptor was being read. The path is reopened via its already-pinned
// parent descriptor; a renamed/replaced leaf must not impersonate the
// original version. This is a best-effort consistency check, not a lock
// against a malicious LOCAL writer with direct filesystem access.
func remoteFileWindowSnapshotStable(before,after os.FileInfo,readLength int)bool{
 return before!=nil&&after!=nil&&before.Mode().IsRegular()&&
  after.Mode().IsRegular()&&before.Size()==int64(readLength)&&
  before.Size()==after.Size()&&before.Mode()==after.Mode()&&
  before.ModTime().Equal(after.ModTime())&&os.SameFile(before,after)
}

// readRemoteFileWindow returns exactly one bounded window of an EXISTING
// visible regular file in the host-primary workspace. Reading the whole
// version into a bounded local buffer permits a whole-file SHA-256: a native
// client can reject version changes between windows or login sessions.
// Nothing is written. No arbitrary OS paths or hidden local TaskDeck state.
func (s *Server) readRemoteFileWindow(ctx context.Context,requestPath string,offset int64)(remotetransport.FileWindow,error){
 var denied=remotetransport.ErrRemoteActionDenied
 var empty remotetransport.FileWindow
 if s==nil||ctx==nil||ctx.Err()!=nil||
  offset<0||offset>remotetransport.RemoteFileWindowMaxSize{
  return empty,denied
 }
 _,path,root,err:=s.restrictedSourcePath(requestPath)
 if err!=nil{return empty,denied}
 pinned,err:=openProjectPinnedFile(root,path)
 if err!=nil{return empty,denied}
 defer pinned.close()
 data,info,err:=pinned.readCurrent(remotetransport.RemoteFileWindowMaxSize)
 // readCurrent performs a bounded descriptor-pinned read, but a concurrent
 // local write/truncate/rename could otherwise produce a partial digest.
 defer func(){for i:=range data{data[i]=0}}()
 if err!=nil||info==nil||!info.Mode().IsRegular()||ctx.Err()!=nil{
  return empty,denied
 }
 current,err:=pinned.openCurrent()
 if err!=nil{return empty,denied}
 latest,statErr:=current.Stat()
 closeErr:=current.Close()
 if statErr!=nil||closeErr!=nil||
  !remoteFileWindowSnapshotStable(info,latest,len(data))||ctx.Err()!=nil{
  return empty,denied
 }
 if offset>int64(len(data)){return empty,denied}
 digest:=sha256.Sum256(data)
 end:=offset+remotetransport.RemoteFileWindowMaxBytes
 if end>int64(len(data)){end=int64(len(data))}
 chunk:=append([]byte(nil),data[int(offset):int(end)]...)
 return remotetransport.FileWindow{
  Offset:offset,Size:int64(len(data)),
  FileSHA256:hex.EncodeToString(digest[:]),Data:chunk,
 },nil
}
