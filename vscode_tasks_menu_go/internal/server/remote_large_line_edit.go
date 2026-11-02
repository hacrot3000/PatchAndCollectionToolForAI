package server

import (
 "bytes"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "os"
 "time"
 "unicode/utf8"

 "bletonfc/vscode_tasks_menu/internal/identity"
 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

// Replace exactly one existing source line. Keep original LF/CRLF and all
// other bytes unchanged. Newline, path and line number are not commands.
// No new file or arbitrary byte-range edit is permitted.
func remoteReplaceExistingSourceLine(source []byte,number int64,replacement string)([]byte,bool){
 if number<1||number>remotetransport.RemoteFileWindowMaxSize||
  !remotetransport.ValidRemoteLargeLineText(replacement)||
  len(source)==0||int64(len(source))>remotetransport.RemoteFileWindowMaxSize||
  !utf8.Valid(source)||bytes.IndexByte(source,0)>=0{return nil,false}
 current:=int64(1)
 for start:=0;start<len(source);{
  end:=len(source)
  next:=len(source)
  if pos:=bytes.IndexByte(source[start:],10);pos>=0{
   end=start+pos
   next=end+1
  }
  if current==number{
   contentEnd:=end
   if end>start&&source[end-1]=='\r'{contentEnd--}
   // The source line must be fully previewable through view-lines
   // (a huge single line cannot be authenticated in a 4KiB page).
   if contentEnd-start>3500{return nil,false}
   out:=make([]byte,0,len(source)-(contentEnd-start)+len(replacement))
   out=append(out,source[:start]...)
   out=append(out,replacement...)
   out=append(out,source[contentEnd:]...)
   if int64(len(out))>remotetransport.RemoteFileWindowMaxSize||
    bytes.Equal(out,source){return nil,false}
   return out,true
  }
  current++
  start=next
 }
 return nil,false
}


// Delete precisely one COMPLETE existing source line. Delete its newline
// separator when present; do not normalize any surviving LF/CRLF bytes.
// A final unterminated line also loses its preceding LF/CRLF delimiter.
func remoteDeleteExistingSourceLine(source []byte,number int64)([]byte,bool){
 if number<1||number>remotetransport.RemoteFileWindowMaxSize||
  len(source)==0||int64(len(source))>remotetransport.RemoteFileWindowMaxSize||
  !utf8.Valid(source)||bytes.IndexByte(source,0)>=0{return nil,false}
 current:=int64(1)
 for start:=0;start<len(source);{
  stop:=len(source)
  if at:=bytes.IndexByte(source[start:],10);at>=0{stop=start+at+1}
  if current==number{
   // The source line must have been fully visible in its E2EE page.
   lineBytes:=stop-start
   if source[stop-1]==10{lineBytes--}
   if lineBytes>0&&source[start+lineBytes-1]=='\r'{lineBytes--}
   if lineBytes>3500{return nil,false}
   // Match the existing small-file delete-line EOF rule: when the
   // terminal line is unterminated, remove its preceding LF/CRLF too.
   // Otherwise the deletion would create a phantom trailing newline.
   removeFrom:=start
   if stop==len(source)&&source[stop-1]!=10&&start>0{
    removeFrom--
    if removeFrom>0&&source[removeFrom-1]=='\r'{removeFrom--}
   }
   out:=make([]byte,0,len(source)-(stop-removeFrom))
   out=append(out,source[:removeFrom]...)
   out=append(out,source[stop:]...)
   return out,true
  }
  current++
  start=stop
 }
 return nil,false
}

// TaskDeck host alone owns this side effect; Portal, Relay and native client
// provide no filesystem handle or executable command. Review SHA is obtained
// from a full-file hash of an authenticated line-page, not from PHP.
func (s *Server) remoteEditLargeFileLine(ctx context.Context,session *remoteTaskDeckSession,
 channel *remotetransport.Channel,request remotetransport.RemoteAction)(string,error){
 denied:=remotetransport.ErrRemoteActionDenied
 if s==nil||ctx==nil||ctx.Err()!=nil||session==nil||channel==nil||
  (!remotetransport.ValidRemoteLargeLineText(request.Content)||
   (request.Operation==remotetransport.RemoteActionProjectFileDeleteLine&&request.Content!=""))||
  request.Offset<1||request.Offset>remotetransport.RemoteFileWindowMaxSize{return "",denied}
 if err:=s.authorizeRemoteTransportAction(ctx,session,channel,identity.PermissionFilesRead);err!=nil{return "",denied}
 relative,err:=cleanProjectRelativePath(request.Path,false)
 if err!=nil||relative!=request.Path{return "",denied}
 if s.Config.SharedServerEnabled{
  s.refreshSharedMutationLock()
  lease,conflict,err:=s.sharedMutation.acquire(identity.Principal{
   UserID:session.userID,Username:session.clientUsername,
  },"file.write",relative,time.Now().UTC())
  if err!=nil||conflict!=nil{return "",denied}
  defer s.releaseSharedMutation(lease)
 }
 path,err:=s.resolveProjectPath(relative,false,false)
 if err!=nil{return "",denied}
 rootView,_,err:=s.projectRootForVirtualPath(relative)
 if err!=nil{return "",denied}
 root,err:=s.projectRoot()
 if err!=nil||rootView.Path!=root{return "",denied}
 pinned,err:=openProjectPinnedFile(root,path)
 if err!=nil{return "",denied}
 defer pinned.close()
 before,info,err:=pinned.readCurrent(remotetransport.RemoteFileWindowMaxSize)
 defer func(){for i:=range before{before[i]=0}}()
 if err!=nil||info==nil||!info.Mode().IsRegular()||
  info.Mode().Perm()&0o222==0||!projectTextBytesValid(before){return "",denied}
 expected:=sha256.Sum256(before)
 if hex.EncodeToString(expected[:])!=request.ExpectedSHA256{return "",denied}
 var next []byte
 var ok bool
 auditAction:="project_file_edit_line"
 if request.Operation==remotetransport.RemoteActionProjectFileDeleteLine{
  next,ok=remoteDeleteExistingSourceLine(before,request.Offset)
  auditAction="project_file_delete_line"
 }else{
  next,ok=remoteReplaceExistingSourceLine(before,request.Offset,request.Content)
 }
 if !ok{return "",denied}
 defer func(){for i:=range next{next[i]=0}}()

 // A failed durable local audit prevents all tempfile and file writes.
 if err:=s.auditRemoteReadOnlyAction(session,auditAction);err!=nil{return "",denied}
 for _,right:=range []string{identity.PermissionFilesRead,identity.PermissionFilesWrite}{
  if err:=s.authorizeRemoteTransportAction(ctx,session,channel,right);err!=nil{return "",denied}
 }
 session.executionMu.Lock()
 defer session.executionMu.Unlock()
 if session.closed.Load()||session.channel!=channel||session.portalGrant==nil||
  !session.portalGrant.ActiveForSession(channel)||ctx.Err()!=nil{return "",denied}

 latest,latestInfo,err:=pinned.readCurrent(remotetransport.RemoteFileWindowMaxSize)
 defer func(){for i:=range latest{latest[i]=0}}()
 if err!=nil||latestInfo==nil||!os.SameFile(info,latestInfo)||
  !remoteFileWindowSnapshotStable(info,latestInfo,len(before))||
  !bytes.Equal(latest,before){return "",denied}
 if err:=pinned.writeTemp(next,latestInfo);err!=nil{return "",denied}
 // Even while preparing a 2MiB tempfile, an external editor can change
 // the source. Revalidate inode, timestamps and all bytes before rename.
 final,finalInfo,err:=pinned.readCurrent(remotetransport.RemoteFileWindowMaxSize)
 defer func(){for i:=range final{final[i]=0}}()
 if err!=nil||finalInfo==nil||!os.SameFile(latestInfo,finalInfo)||
  !remoteFileWindowSnapshotStable(latestInfo,finalInfo,len(latest))||
  !bytes.Equal(final,latest)||ctx.Err()!=nil{return "",denied}
 if err:=pinned.commitTemp();err!=nil{return "",denied}
 digest:=sha256.Sum256(next)
 return hex.EncodeToString(digest[:]),nil
}
