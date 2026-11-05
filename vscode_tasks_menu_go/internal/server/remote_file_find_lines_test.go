package server

import (
 "bytes"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "bletonfc/vscode_tasks_menu/internal/remotetransport"
)

func TestRemoteFileFindLinesFindsBeyondPreviewAndIncludesAllMatches(t *testing.T){
 s,_,_,_:=remoteDispatchFixtureWithRole(t,remotetransport.RoleAdmin)
 var contents strings.Builder
 for i:=1;i<=400;i++{
  if i==330||i==395{contents.WriteString("needle intact source line ✓\n")
  }else{contents.WriteString("ordinary unrelated line\n")}
 }
 data:=[]byte(contents.String())
 if len(data)<=4096{t.Fatal("fixture smaller than preview")}
 if err:=os.WriteFile(filepath.Join(s.Workspace,"large.txt"),data,0600);err!=nil{t.Fatal(err)}
 result,err:=s.readRemoteFileFindLines(context.Background(),"large.txt","needle")
 if err!=nil{t.Fatal(err)}
 h:=sha256.Sum256(data)
 want:=hex.EncodeToString(h[:])
 if !bytes.Contains(result,[]byte("FILE_SHA256 "+want+"\n"))||
  !bytes.Contains(result,[]byte("TOTAL_LINES 400\nMATCHES 2\n"))||
  !bytes.Contains(result,[]byte("330: needle intact source line"))||
  !bytes.Contains(result,[]byte("395: needle intact source line"))||
  len(result)>remotetransport.RemoteGitDiffMaxBytes{
  t.Fatalf("search returned partial/incorrect matches %q",result)
 }
 absent,err:=s.readRemoteFileFindLines(context.Background(),"large.txt","neverfound")
 if err!=nil||!bytes.Contains(absent,[]byte("MATCHES 0\n")){t.Fatalf("zero matches rejected: %v %q",err,absent)}
 fresh,err:=os.ReadFile(filepath.Join(s.Workspace,"large.txt"))
 if err!=nil||!bytes.Equal(data,fresh){t.Fatal("search modified source")}
 if err:=os.WriteFile(filepath.Join(s.Workspace,"large.txt"),[]byte("needle new revision\n"),0600);err!=nil{t.Fatal(err)}
 changed,err:=s.readRemoteFileFindLines(context.Background(),"large.txt","needle")
 if err!=nil||bytes.Contains(changed,[]byte(want)){t.Fatal("search reused stale file SHA")}
}
func TestRemoteFileFindLinesRefusesPartialResultAndUnsafeFiles(t *testing.T){
 s,_,_,_:=remoteDispatchFixtureWithRole(t,remotetransport.RoleAdmin)
 cases:=[]struct{name string;data []byte}{
  {"many.txt",[]byte(strings.Repeat("needle here\n",900))},
  {"huge.txt",[]byte("needle "+strings.Repeat("x",4100)+"\n")},
  {"binary.txt",[]byte{0xff,0xfe,0}},
  {"nul.txt",[]byte("needle\x00hidden")},
 }
 for _,tc:=range cases{
  if err:=os.WriteFile(filepath.Join(s.Workspace,tc.name),tc.data,0600);err!=nil{t.Fatal(err)}
  if result,err:=s.readRemoteFileFindLines(context.Background(),tc.name,"needle");err==nil||len(result)>0{
   t.Fatalf("%s returned a partial/unsafe source search",tc.name)
  }
 }
 for _,bad:=range []string{"../private",".git/config","missing"}{
  if result,err:=s.readRemoteFileFindLines(context.Background(),bad,"needle");err==nil||len(result)>0{
   t.Fatalf("unsafe path %s accepted",bad)
  }
 }
}

func TestRemoteFileFindLinesRequiresLocalPrincipalAndLiveSession(t *testing.T){
 s,session,client,host:=remoteDispatchFixtureWithRole(t,remotetransport.RoleAdmin)
 if err:=os.WriteFile(filepath.Join(s.Workspace,"safe.txt"),[]byte("one\nneedle\n"),0600);err!=nil{t.Fatal(err)}
 request,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionProjectFileFindLines,
  Path:"safe.txt",Content:"needle",
 })
 if err!=nil{t.Fatal(err)}
 reply,err:=s.executeRemoteE2EEPing(context.Background(),nil,host,request)
 if err==nil||len(reply)>0{t.Fatal("PHP-alone request read source without TaskDeck login")}
 reply,err=s.executeRemoteE2EEPing(context.Background(),session,host,request)
 if err!=nil{t.Fatal(err)}
 text,err:=remotetransport.OpenRemoteFindLinesResult(client,reply,remoteDispatchTestID)
 if err!=nil||!strings.Contains(text,"2: needle\n"){t.Fatalf("E2EE result wrong %v %q",err,text)}
 if _,err:=s.executeRemoteE2EEPing(context.Background(),session,host,request);err==nil{
  t.Fatal("replayed encrypted file search action accepted")
 }
}
