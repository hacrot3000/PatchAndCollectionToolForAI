package server

import (
 "bytes"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "fmt"
 "strings"
 "unicode/utf8"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// Complete literal matches, or NOTHING: no truncated or omitted matches.
// A source <=2 MiB is opened via pinned parent/leaf descriptors, checked
// before and after the read. Git metadata, hidden files and traversal
// remain denied by the restrictedSourcePath host boundary.
func (s *Server) readRemoteFileFindLines(ctx context.Context,pathArg,literal string)([]byte,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if s==nil||ctx==nil||ctx.Err()!=nil||
  !remotetransport.ValidRemoteFindLiteral(literal){return nil,denied}
 _,path,root,err:=s.restrictedSourcePath(pathArg)
 if err!=nil{return nil,denied}
 pinned,err:=openProjectPinnedFile(root,path)
 if err!=nil{return nil,denied}
 defer pinned.close()
 data,info,err:=pinned.readCurrent(remotetransport.RemoteFileWindowMaxSize)
 defer func(){for i:=range data{data[i]=0}}()
 if err!=nil||info==nil||!info.Mode().IsRegular()||
  !utf8.Valid(data)||bytes.IndexByte(data,0)>=0||ctx.Err()!=nil{
  return nil,denied
 }
 current,err:=pinned.openCurrent()
 if err!=nil{return nil,denied}
 after,statErr:=current.Stat()
 closeErr:=current.Close()
 if statErr!=nil||closeErr!=nil||
  !remoteFileWindowSnapshotStable(info,after,len(data))||
  ctx.Err()!=nil{return nil,denied}
 rows:=bytes.Split(data,[]byte{10})
 total:=len(rows)
 if len(data)==0{total=0}
 if len(data)>0&&data[len(data)-1]==10{total--}
 var matches strings.Builder
 count:=0
 for i:=0;i<total;i++{
  if ctx.Err()!=nil{return nil,denied}
  row:=string(rows[i])
  if !strings.Contains(row,literal){continue}
  // The whole line must fit. Never display a misleading clipped hit.
  entry:=fmt.Sprintf("%d: %s\n",i+1,row)
  if len(entry)+matches.Len()>3600{return nil,denied}
  matches.WriteString(entry)
  count++
 }
 fileSHA:=sha256.Sum256(data)
 termSHA:=sha256.Sum256([]byte(literal))
 report:=fmt.Sprintf(
  "FILE_SHA256 %s\nTERM_SHA256 %s\nTOTAL_LINES %d\nMATCHES %d\n-- COMPLETE MATCHING LINES --\n%s",
  hex.EncodeToString(fileSHA[:]),hex.EncodeToString(termSHA[:]),
  total,count,matches.String(),
 )
 if len(report)>remotetransport.RemoteGitDiffMaxBytes||
  !utf8.ValidString(report)||ctx.Err()!=nil{return nil,denied}
 return []byte(report),nil
}
