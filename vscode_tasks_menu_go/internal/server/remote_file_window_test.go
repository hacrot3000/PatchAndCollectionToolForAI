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

func TestRemoteFileWindowE2EEAuthorizedVersionPinnedBinaryRead(t *testing.T){
 s,session,native,agent:=remoteDispatchFixture(t)
 dir:=filepath.Join(s.Workspace,"visible")
 if err:=os.Mkdir(dir,0700);err!=nil{t.Fatal(err)}
 file:=filepath.Join(dir,"data.bin")
 data:=make([]byte,400)
 for i:=range data{data[i]=byte(i%256)}
 if err:=os.WriteFile(file,data,0600);err!=nil{t.Fatal(err)}
 reqID:=remoteDispatchTestID
 request,err:=remotetransport.SealRemoteAction(native,remotetransport.RemoteAction{
  RequestID:reqID,Operation:remotetransport.RemoteActionProjectFileWindow,
  Path:"visible/data.bin",Offset:128,
 })
 if err!=nil{t.Fatal(err)}
 if reply,err:=s.executeRemoteE2EEPing(context.Background(),nil,agent,request);err==nil||len(reply)>0{
  t.Fatal("compromised Portal-only grant read file without local TaskDeck principal")
 }
 reply,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,request)
 if err!=nil{t.Fatal(err)}
 window,err:=remotetransport.OpenRemoteFileWindowResult(native,reply,reqID,128)
 digest:=sha256.Sum256(data)
 if err!=nil||window.Offset!=128||window.Size!=400||
  window.FileSHA256!=hex.EncodeToString(digest[:])||
  !bytes.Equal(window.Data,data[128:256]){
  t.Fatalf("file window content/version changed: %+v %v",window,err)
 }
 if again,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,request);err==nil||len(again)>0{
  t.Fatal("replayed window read accepted")
 }
}

func TestRemoteFileWindowLimitsNoEscapeAndVersionHashChange(t *testing.T){
 s,_,_,_:=remoteDispatchFixture(t)
 data:=bytes.Repeat([]byte("abcd"),200)
 file:=filepath.Join(s.Workspace,"visible-binary.txt")
 if err:=os.WriteFile(file,data,0600);err!=nil{t.Fatal(err)}
 first,err:=s.readRemoteFileWindow(context.Background(),"visible-binary.txt",0)
 if err!=nil{t.Fatal(err)}
 second,err:=s.readRemoteFileWindow(context.Background(),"visible-binary.txt",128)
 if err!=nil||second.FileSHA256!=first.FileSHA256||
  !bytes.Equal(second.Data,data[128:256]){t.Fatalf("window offset inconsistent: %+v %v",second,err)}
 data[0]='Z'
 if err:=os.WriteFile(file,data,0600);err!=nil{t.Fatal(err)}
 changed,err:=s.readRemoteFileWindow(context.Background(),"visible-binary.txt",128)
 if err!=nil||changed.FileSHA256==first.FileSHA256{
  t.Fatalf("client cannot detect changed file across windows: %+v %v",changed,err)
 }
 for _,path:=range []string{".git/config",".vscode/tasks.json","../outside",".","@root/other/file"}{
  if _,err:=s.readRemoteFileWindow(context.Background(),path,0);err==nil{
   t.Fatalf("hidden/outside workspace path disclosed: %q",path)
  }
 }
 target:=filepath.Join(t.TempDir(),"secret")
 if err:=os.WriteFile(target,[]byte("external"),0600);err!=nil{t.Fatal(err)}
 if err:=os.Symlink(target,filepath.Join(s.Workspace,"exfil"));err!=nil{t.Fatal(err)}
 if _,err:=s.readRemoteFileWindow(context.Background(),"exfil",0);err==nil{
  t.Fatal("window followed workspace symlink outside project")
 }
 if _,err:=s.readRemoteFileWindow(context.Background(),"visible-binary.txt",801);err==nil{
  t.Fatal("window beyond EOF returned data")
 }
 if err:=os.WriteFile(file,make([]byte,(2<<20)+1),0600);err!=nil{t.Fatal(err)}
 if _,err:=s.readRemoteFileWindow(context.Background(),"visible-binary.txt",0);err==nil{
  t.Fatal("oversized file bypassed 2 MiB memory cap")
 }
}

func TestRemoteNativeFileWindowClientChecksFullFileIdentity(t *testing.T){
 s,session,native,agent:=remoteDispatchFixture(t)
 path:=filepath.Join(s.Workspace,"remote-window.bin")
 contents:=make([]byte,256)
 for i:=range contents{contents[i]=byte(i)}
 if err:=os.WriteFile(path,contents,0600);err!=nil{t.Fatal(err)}
 client,err:=remotetransport.NewPingClient(native,localAuthenticatedPingLink{s,session,agent})
 if err!=nil{t.Fatal(err)}
 defer client.Close()
 first,err:=client.ReadFileWindow(context.Background(),"remote-window.bin",0)
 if err!=nil||first.Offset!=0||first.Size!=256||!bytes.Equal(first.Data,contents[:128]){
  t.Fatalf("native initial E2EE window incorrect: %+v %v",first,err)
 }
 second,err:=client.ReadFileWindow(context.Background(),"remote-window.bin",128)
 if err!=nil||second.FileSHA256!=first.FileSHA256||
  !bytes.Equal(second.Data,contents[128:]){t.Fatalf("native next window incorrect: %+v %v",second,err)}
}

func TestRemoteFileWindowRevokedApprovedKeyClosesSession(t *testing.T){
 s,session,native,agent:=remoteDispatchFixture(t)
 session.clientProofVerified=false
 req,err:=remotetransport.SealRemoteAction(native,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionProjectFileWindow,
  Path:"visible/data.bin",Offset:0,
 })
 if err!=nil{t.Fatal(err)}
 if result,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req);
  err==nil||len(result)>0||!session.closed.Load(){
  t.Fatal("revoked host-approved client proof retained remote FilesRead")
 }
}
