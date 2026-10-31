package server

import (
 "bytes"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "fmt"
 "os"
 "path/filepath"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func TestRemoteScopedTransferOnlyOneFileAndNeverTaskRunOrOtherOperations(t *testing.T){
 s,session,client,agent:=remoteDispatchFixture(t)
 fileData:=bytes.Repeat([]byte("A"),12000)
 if err:=os.WriteFile(filepath.Join(s.Workspace,"read-only.bin"),fileData,0600);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(s.Workspace,"another.bin"),[]byte("another"),0600);err!=nil{t.Fatal(err)}
 request:=func(n int,op,path string,offset int64)([]byte,string){
  id:=fmt.Sprintf("%032x",n)
  frame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
   RequestID:id,Operation:op,Path:path,Offset:offset,
  })
  if err!=nil{t.Fatal(err)}
  return frame,id
 }
 first,id:=request(1,remotetransport.RemoteActionProjectFileChunk,"read-only.bin",0)
 result,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,first)
 if err!=nil{t.Fatal(err)}
 window,err:=remotetransport.OpenRemoteFileChunkResult(client,result,id,0)
 if err!=nil||len(window.Data)!=4096||!session.fileTransferActive(){
  t.Fatalf("first FilesRead did not irreversibly enter transfer mode: %v",err)
 }
 for i,c:=range []struct{op,path string;offset int64}{
  {remotetransport.RemoteActionSessionPing,"",0},
  {remotetransport.RemoteActionProjectFileRead,"read-only.bin",0},
  {remotetransport.RemoteActionProjectFileChunk,"another.bin",4096},
  {remotetransport.RemoteActionProjectFileChunk,"read-only.bin",0},
  {remotetransport.RemoteActionProjectFileChunk,"read-only.bin",8192},
 }{
  frame,_:=request(i+2,c.op,c.path,c.offset)
  if data,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,frame);
   err==nil||len(data)>0{
   t.Fatalf("file-only mode permitted operation=%s path=%s offset=%d",
    c.op,c.path,c.offset)
  }
 }
 second,id:=request(20,remotetransport.RemoteActionProjectFileChunk,"read-only.bin",4096)
 result,err=s.executeRemoteE2EEPing(context.Background(),session,agent,second)
 if err!=nil{t.Fatalf("valid next window denied: %v",err)}
 window,err=remotetransport.OpenRemoteFileChunkResult(client,result,id,4096)
 if err!=nil||len(window.Data)!=4096{t.Fatalf("wrong second chunk: %v",err)}
 // Changing the same FILE behind the descriptor is not sufficient to pass:
 // every subsequent result must match the full-file snapshot SHA.
 changed:=bytes.Repeat([]byte("B"),12000)
 if err:=os.WriteFile(filepath.Join(s.Workspace,"read-only.bin"),changed,0600);err!=nil{t.Fatal(err)}
 third,_:=request(21,remotetransport.RemoteActionProjectFileChunk,"read-only.bin",8192)
 if data,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,third);
  err==nil||len(data)>0{t.Fatal("same-path new SHA was accepted into pinned transfer")}
}

func TestScopedFileTransferLocalQuotaIsIndependentOfRelay(t *testing.T){
 session:=&remoteTaskDeckSession{}
 if session.fileTransferActive(){t.Fatal("fresh password session auto-upgraded")}
 const length=int64(2<<20)
 sum:=sha256.Sum256([]byte("host version"))
 sha:=hex.EncodeToString(sum[:])
 offset:=int64(0)
 for i:=0;i<remotetransport.MaxScopedFileTransferRounds;i++{
  if err:=session.checkScopedFileTransfer(remotetransport.RemoteActionProjectFileChunk,
   "same-file.bin",offset);err!=nil{t.Fatal(err)}
  window:=remotetransport.FileWindow{
   Offset:offset,Size:length,FileSHA256:sha,Data:make([]byte,4096),
  }
  if err:=session.commitScopedFileTransfer(
   remotetransport.RemoteActionProjectFileChunk,"same-file.bin",offset,window);err!=nil{
   t.Fatalf("local quota rejected round %d: %v",i,err)
  }
  offset+=4096
 }
 if !session.fileTransferActive(){t.Fatal("transfer flag lost after 64 rounds")}
 if err:=session.checkScopedFileTransfer(remotetransport.RemoteActionProjectFileChunk,
  "same-file.bin",offset);err==nil{t.Fatal("compromised Relay bypassed host 64-round quota")}
 if err:=session.checkScopedFileTransfer(remotetransport.RemoteActionTaskRun,
  "",0);err==nil{t.Fatal("TaskRun allowed after authorized file transfer")}
 if err:=session.checkScopedFileTransfer(remotetransport.RemoteActionProjectFileReplace,
  "same-file.bin",0);err==nil{t.Fatal("FilesWrite allowed after authorized file transfer")}
}
