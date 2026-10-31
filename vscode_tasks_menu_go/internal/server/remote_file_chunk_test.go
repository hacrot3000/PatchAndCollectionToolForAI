package server

import (
 "bytes"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "os"
 "path/filepath"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func TestRemoteFileChunkRealTaskDeckPrincipalAndVersion(t *testing.T){
 s,session,client,agent:=remoteDispatchFixture(t)
 payload:=make([]byte,9200)
 for i:=range payload{payload[i]=byte(i%255)}
 if err:=os.WriteFile(filepath.Join(s.Workspace,"large-public.bin"),payload,0600);err!=nil{t.Fatal(err)}
 req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionProjectFileChunk,
  Path:"large-public.bin",Offset:4096,
 })
 if err!=nil{t.Fatal(err)}
 if out,err:=s.executeRemoteE2EEPing(context.Background(),nil,agent,req);
  err==nil||len(out)>0{t.Fatal("hacked PHP grant read file without native TaskDeck login")}
 response,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req)
 if err!=nil{t.Fatal(err)}
 result,err:=remotetransport.OpenRemoteFileChunkResult(client,response,remoteDispatchTestID,4096)
 digest:=sha256.Sum256(payload)
 if err!=nil||result.FileSHA256!=hex.EncodeToString(digest[:])||
  result.Size!=int64(len(payload))||!bytes.Equal(result.Data,payload[4096:8192]){
  t.Fatalf("wrong pinned 4096-byte file result: %v len=%d",err,len(result.Data))
 }
 if out,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req);
  err==nil||len(out)>0{t.Fatal("bulk request replay accepted")}
}

func TestRemoteFileChunkDeniedHiddenSymlinkOversizeAndRevokedKey(t *testing.T){
 s,session,client,agent:=remoteDispatchFixture(t)
 outside:=filepath.Join(t.TempDir(),"outside")
 if err:=os.WriteFile(outside,[]byte("secret"),0600);err!=nil{t.Fatal(err)}
 if err:=os.Symlink(outside,filepath.Join(s.Workspace,"escape.bin"));err!=nil{t.Fatal(err)}
 for _,path:=range []string{"escape.bin",".vscode/tasks.json",".git/config","../outside"}{
  if _,err:=s.readRemoteFileChunk(context.Background(),path,0);err==nil{
   t.Fatalf("remote chunk reader escaped visible primary workspace: %q",path)
  }
 }
 tooBig:=filepath.Join(s.Workspace,"over-limit.bin")
 if err:=os.WriteFile(tooBig,make([]byte,(2<<20)+1),0600);err!=nil{t.Fatal(err)}
 if _,err:=s.readRemoteFileChunk(context.Background(),"over-limit.bin",0);err==nil{
  t.Fatal("remote chunk read oversized 2 MiB file")
 }
 session.clientProofVerified=false
 req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionProjectFileChunk,
  Path:"over-limit.bin",Offset:0,
 })
 if err!=nil{t.Fatal(err)}
 if out,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req);
  err==nil||len(out)>0||!session.closed.Load(){
  t.Fatal("revoked client key authenticated 4KiB read")
 }
}
