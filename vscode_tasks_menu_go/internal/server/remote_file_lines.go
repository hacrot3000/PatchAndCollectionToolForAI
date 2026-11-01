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

// Read-only page of COMPLETE source lines for the native interactive console.
// This never enables the scoped binary download / file-transfer mode.
// The SHA-256 covers the ENTIRE pinned file (max 2 MiB) so the client
// detects revisions between independent 4KiB E2EE page requests.
func (s *Server) readRemoteFileLines(ctx context.Context,requestPath string,start int64)([]byte,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if s==nil||ctx==nil||ctx.Err()!=nil||start<1||
  start>remotetransport.RemoteFileWindowMaxSize{return nil,denied}
 _,path,root,err:=s.restrictedSourcePath(requestPath)
 if err!=nil{return nil,denied}
 pinned,err:=openProjectPinnedFile(root,path)
 if err!=nil{return nil,denied}
 defer pinned.close()
 data,info,err:=pinned.readCurrent(remotetransport.RemoteFileWindowMaxSize)
 defer func(){for i:=range data{data[i]=0}}()
 if err!=nil||info==nil||!info.Mode().IsRegular()||ctx.Err()!=nil||
  !utf8.Valid(data)||bytes.IndexByte(data,0)>=0{return nil,denied}
 current,err:=pinned.openCurrent()
 if err!=nil{return nil,denied}
 after,statErr:=current.Stat()
 closeErr:=current.Close()
 if statErr!=nil||closeErr!=nil||
  !remoteFileWindowSnapshotStable(info,after,len(data))||
  ctx.Err()!=nil{return nil,denied}
 // Preserve the complete original byte sequence for SHA validation,
 // including CRLF. Only line separators delimit displayed page rows.
 rows:=bytes.Split(data,[]byte{10})
 count:=len(rows)
 if len(data)==0 {count=0}
 if len(data)>0&&data[len(data)-1]==10{count--}
 if count==0||start>int64(count){return nil,denied}
 sum:=sha256.Sum256(data)
 var body strings.Builder
 next:=int64(0)
 for i:=int(start-1);i<count;i++{
  line:=fmt.Sprintf("%d: %s\n",i+1,rows[i])
  // Reserve 256 bytes for the authenticated header and metadata.
  // Never truncate an individual source line or conceal omitted rows.
  if body.Len()+len(line)>3800{
   if body.Len()==0{return nil,denied}
   next=int64(i+1)
   break
  }
  body.WriteString(line)
 }
 report:=fmt.Sprintf(
  "FILE_SHA256 %s\nSTART_LINE %d\nNEXT_LINE %d\nTOTAL_LINES %d\n-- COMPLETE SOURCE LINES --\n%s",
  hex.EncodeToString(sum[:]),start,next,count,body.String(),
 )
 if len(report)>remotetransport.RemoteGitDiffMaxBytes||
  !utf8.ValidString(report)||ctx.Err()!=nil{return nil,denied}
 return []byte(report),nil
}
