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

func TestFileBlockRealHostFilesReadAndStrictVersion(t *testing.T){
 s,session,client,agent:=remoteDispatchFixture(t)
 original:=make([]byte,2<<20)
 for i:=range original{original[i]=byte(i%253)}
 source:=filepath.Join(s.Workspace,"large-e2ee-block.bin")
 if err:=os.WriteFile(source,original,0600);err!=nil{t.Fatal(err)}
 request:=func(i int,op string,path string,offset int64)([]byte,string){
  id:=fmt.Sprintf("%032x",i)
  frame,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
   RequestID:id,Operation:op,Path:path,Offset:offset,
  })
  if err!=nil{t.Fatal(err)}
  return frame,id
 }
 first,id:=request(1,remotetransport.RemoteActionProjectFileBlock,"large-e2ee-block.bin",0)
 sealed,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,first)
 if err!=nil{t.Fatal(err)}
 window,err:=remotetransport.OpenRemoteFileBlockResult(client,sealed,id,0)
 digest:=sha256.Sum256(original)
 if err!=nil||window.Size!=int64(len(original))||window.FileSHA256!=hex.EncodeToString(digest[:])||
  !bytes.Equal(window.Data,original[:32768])||!session.fileTransferActive(){
  t.Fatalf("invalid file-only 32KiB first block: %v",err)
 }
 for i,c:=range []struct{op,path string;offset int64}{
  {remotetransport.RemoteActionProjectFileChunk,"large-e2ee-block.bin",32768},
  {remotetransport.RemoteActionProjectFileRead,"large-e2ee-block.bin",0},
  {remotetransport.RemoteActionProjectFileBlock,"other-file.bin",32768},
  {remotetransport.RemoteActionProjectFileBlock,"large-e2ee-block.bin",65536},
  {remotetransport.RemoteActionSessionPing,"",0},
 }{
  frame,_:=request(i+2,c.op,c.path,c.offset)
  if data,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,frame);
   err==nil||len(data)>0{
   t.Fatalf("host scoped file block allowed scope escape %q offset %d",c.op,c.offset)
  }
 }
 second,id:=request(22,remotetransport.RemoteActionProjectFileBlock,"large-e2ee-block.bin",32768)
 sealed,err=s.executeRemoteE2EEPing(context.Background(),session,agent,second)
 if err!=nil{t.Fatal(err)}
 window,err=remotetransport.OpenRemoteFileBlockResult(client,sealed,id,32768)
 if err!=nil||!bytes.Equal(window.Data,original[32768:65536]){t.Fatalf("wrong sequential block: %v",err)}
 altered:=bytes.Repeat([]byte("B"),2<<20)
 if err:=os.WriteFile(source,altered,0600);err!=nil{t.Fatal(err)}
 next,_:=request(23,remotetransport.RemoteActionProjectFileBlock,"large-e2ee-block.bin",65536)
 if data,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,next);
  err==nil||len(data)>0{t.Fatal("changed file SHA escaped immutable host transfer version")}
}

func TestFileBlockHostCapsEntire2MiBAt64AuthenticatedReadOnlyRounds(t *testing.T){
 session:=&remoteTaskDeckSession{}
 const fileSize=int64(2<<20)
 sha:=hex.EncodeToString(bytes.Repeat([]byte{0xa5},32))
 offset:=int64(0)
 for i:=0;i<remotetransport.MaxScopedFileTransferRounds;i++{
  if err:=session.checkScopedFileTransfer(
   remotetransport.RemoteActionProjectFileBlock,"exact-2MiB.bin",offset);err!=nil{
   t.Fatalf("blocked correct 2MiB read %d: %v",i,err)
  }
  w:=remotetransport.FileWindow{Offset:offset,Size:fileSize,FileSHA256:sha,
   Data:make([]byte,remotetransport.RemoteFileBlockMaxBytes)}
  if err:=session.commitScopedFileTransfer(
   remotetransport.RemoteActionProjectFileBlock,"exact-2MiB.bin",offset,w);err!=nil{
   t.Fatalf("block %d failed: %v",i,err)
  }
  offset+=remotetransport.RemoteFileBlockMaxBytes
 }
 if offset!=fileSize||!session.fileTransferActive(){t.Fatal("2MiB transfer not complete at 64 blocks")}
 if err:=session.checkScopedFileTransfer(
  remotetransport.RemoteActionProjectFileBlock,"exact-2MiB.bin",offset);err==nil{
  t.Fatal("compromised Relay exceeded host budget")
 }
 if err:=session.checkScopedFileTransfer(
  remotetransport.RemoteActionTaskRun,"",0);err==nil{
  t.Fatal("file-only mode permitted arbitrary TaskDeck task")
 }
}
