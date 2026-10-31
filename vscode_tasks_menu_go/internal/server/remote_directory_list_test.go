package server

import (
 "context"
 "errors"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func TestRemoteE2EEDirectoryListingRequiresTaskDeckPrincipalAndFilesRead(t *testing.T){
 s,session,client,agent:=remoteDispatchFixture(t)
 directory:=filepath.Join(s.Workspace,"public")
 if err:=os.Mkdir(directory,0700);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(directory,"example.txt"),[]byte("safe"),0600);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(directory,".private"),[]byte("secret"),0600);err!=nil{t.Fatal(err)}
 if err:=os.Mkdir(filepath.Join(directory,"subdir"),0700);err!=nil{t.Fatal(err)}
 outside:=t.TempDir()
 if err:=os.Symlink(outside,filepath.Join(directory,"external-link"));err!=nil{t.Fatal(err)}
 req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionProjectDirectoryList,Path:"public",
 })
 if err!=nil{t.Fatal(err)}
 if answer,err:=s.executeRemoteE2EEPing(context.Background(),nil,agent,req);err==nil||len(answer)>0{
  t.Fatal("PHP/Relay-only grant listed workspace without TaskDeck login")
 }
 sealed,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req)
 if err!=nil{t.Fatal(err)}
 listing,err:=remotetransport.OpenRemoteDirectoryListResult(client,sealed,remoteDispatchTestID)
 if err!=nil||listing!="F\texample.txt\nD\tsubdir" && listing!="D\tsubdir\nF\texample.txt"{
  t.Fatalf("unexpected bounded directory response: %q %v",listing,err)
 }
 if strings.Contains(listing,".private")||strings.Contains(listing,"external-link")||
  strings.Contains(listing,"secret"){t.Fatal("hidden content or symlink leaked")}
 if again,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req);err==nil||len(again)>0{
  t.Fatal("replayed authenticated directory enumeration admitted")
 }
}

func TestRemoteDirectoryListingProtectsPathsAndSize(t *testing.T){
 t.Run("outside symlink",func(t *testing.T){
  s,session,client,agent:=remoteDispatchFixture(t)
  if err:=os.Symlink(t.TempDir(),filepath.Join(s.Workspace,"exfil"));err!=nil{t.Fatal(err)}
  req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
   RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionProjectDirectoryList,Path:"exfil",
  })
  if err!=nil{t.Fatal(err)}
  if result,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req);err==nil||len(result)>0{
   t.Fatal("directory symlink escaped workspace")
  }
 })
 t.Run("hidden path",func(t *testing.T){
  s,session,client,agent:=remoteDispatchFixture(t)
  req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
   RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionProjectDirectoryList,Path:".vscode",
  })
  if err!=nil{t.Fatal(err)}
  if result,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req);err==nil||len(result)>0{
   t.Fatal("hidden project directory enumerated")
  }
 })
 t.Run("oversize directory",func(t *testing.T){
  s,session,client,agent:=remoteDispatchFixture(t)
  dir:=filepath.Join(s.Workspace,"many")
  if err:=os.Mkdir(dir,0700);err!=nil{t.Fatal(err)}
  for i:=0;i<40;i++{
   if err:=os.WriteFile(filepath.Join(dir,strings.Repeat("a",20)+string(rune('A'+i))),[]byte("ok"),0600);err!=nil{t.Fatal(err)}
  }
  req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
   RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionProjectDirectoryList,Path:"many",
  })
  if err!=nil{t.Fatal(err)}
  if result,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req);err==nil||len(result)>0{
   t.Fatal("partial/overlong directory plaintext left TaskDeck")
  }
 })
 t.Run("revoked key",func(t *testing.T){
  s,session,client,agent:=remoteDispatchFixture(t)
  session.clientProofVerified=false
  req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
   RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionProjectDirectoryList,Path:".",
  })
  if err!=nil{t.Fatal(err)}
  if result,err:=s.executeRemoteE2EEPing(context.Background(),session,agent,req);err==nil||len(result)>0||!session.closed.Load(){
   t.Fatal("lost host-approved client key retained directory access")
  }
 })
}

func TestRemoteDirectoryListLocalBehavior(t *testing.T){
 s,_,_,_:=remoteDispatchFixture(t)
 root,err:=s.readRemoteDirectory(context.Background(),".")
 if err!=nil{t.Fatal(err)}
 if strings.Contains(string(root),".vscode"){t.Fatal("root listing leaked hidden workspace files")}
 empty:=filepath.Join(s.Workspace,"empty-visible")
 if err:=os.Mkdir(empty,0700);err!=nil{t.Fatal(err)}
 content,err:=s.readRemoteDirectory(context.Background(),"empty-visible")
 if err!=nil||len(content)!=0{t.Fatalf("empty directory did not succeed: %q %v",content,err)}
 _,err=s.readRemoteDirectory(context.Background(),"../")
 if !errors.Is(err,remotetransport.ErrRemoteActionDenied){t.Fatal("parent directory escape accepted")}
}
