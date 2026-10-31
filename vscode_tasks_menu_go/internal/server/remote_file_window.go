package server

import (
 "context"
 "crypto/sha256"
 "encoding/hex"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

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
 if err!=nil||info==nil||!info.Mode().IsRegular()||ctx.Err()!=nil{
  return empty,denied
 }
 defer func(){for i:=range data{data[i]=0}}()
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
