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

func TestRemoteFileLinesPagedReadOnlyLargeTextAndHash(t *testing.T){
 s,_,_,_:=remoteDispatchFixtureWithRole(t,remotetransport.RoleAdmin)
 var original strings.Builder
 for i:=0;i<900;i++{original.WriteString("A complete UTF-8 source line ✓\n")}
 data:=[]byte(original.String())
 if len(data)<=4096{t.Fatal("fixture must exceed 4 KiB")}
 name:=filepath.Join(s.Workspace,"source.txt")
 if err:=os.WriteFile(name,data,0600);err!=nil{t.Fatal(err)}
 digest:=sha256.Sum256(data)
 wanted:=hex.EncodeToString(digest[:])
 first,err:=s.readRemoteFileLines(context.Background(),"source.txt",1)
 if err!=nil{t.Fatal(err)}
 if len(first)>remotetransport.RemoteGitDiffMaxBytes||
  !bytes.Contains(first,[]byte("FILE_SHA256 "+wanted))||
  !bytes.Contains(first,[]byte("START_LINE 1\n"))||
  !bytes.Contains(first,[]byte("TOTAL_LINES 900\n"))||
  !bytes.Contains(first,[]byte("1: A complete UTF-8 source line"))||
  !bytes.Contains(first,[]byte("NEXT_LINE ")){
  t.Fatalf("line page incomplete, oversize or wrong source SHA: %q",first)
 }
 last,err:=s.readRemoteFileLines(context.Background(),"source.txt",900)
 if err!=nil{t.Fatal(err)}
 if !bytes.Contains(last,[]byte("NEXT_LINE 0\n"))||
  !bytes.Contains(last,[]byte("900: A complete UTF-8 source line")){
  t.Fatalf("EOF page was truncated or malformed: %q",last)
 }
 latest,err:=os.ReadFile(name)
 if err!=nil||!bytes.Equal(latest,data){t.Fatal("line reader mutated source")}
 // Later pages derive a new full-file SHA after a real local source change.
 if err:=os.WriteFile(name,append(data,[]byte("another line\n")...),0600);err!=nil{t.Fatal(err)}
 after,err:=s.readRemoteFileLines(context.Background(),"source.txt",1)
 if err!=nil||bytes.Contains(after,[]byte("FILE_SHA256 "+wanted)){
  t.Fatal("source revision was not detected by full-file digest")
 }
}

func TestRemoteFileLinesRefusesUnsafeSourceAndIncompleteHugeLine(t *testing.T){
 s,_,_,_:=remoteDispatchFixtureWithRole(t,remotetransport.RoleAdmin)
 for _,tc:=range []struct{file string;data []byte}{
  {"nul.txt",[]byte("safe\x00poison\n")},
  {"binary.txt",[]byte{0xff,0xfe,0x00}},
  {"verylong.txt",[]byte(strings.Repeat("x",4000)+"\n")},
 }{
  if err:=os.WriteFile(filepath.Join(s.Workspace,tc.file),tc.data,0600);err!=nil{t.Fatal(err)}
  if result,err:=s.readRemoteFileLines(context.Background(),tc.file,1);err==nil||len(result)!=0{
   t.Fatalf("invalid source %s rendered partial/unsafe lines",tc.file)
  }
 }
 if result,err:=s.readRemoteFileLines(context.Background(),"../private",1);err==nil||len(result)!=0{
  t.Fatal("relative traversal was accepted")
 }
 if result,err:=s.readRemoteFileLines(context.Background(),"source.txt",0);err==nil||len(result)!=0{
  t.Fatal("zero line number was accepted")
 }
 if result,err:=s.readRemoteFileLines(context.Background(),"source.txt",2<<20+1);err==nil||len(result)!=0{
  t.Fatal("out-of-range line number was accepted")
 }
}

func TestRemoteFileLinesE2EERequiresHostPrincipalAndFilesRead(t *testing.T){
 s,session,client,host:=remoteDispatchFixtureWithRole(t,remotetransport.RoleAdmin)
 if err:=os.WriteFile(filepath.Join(s.Workspace,"source.txt"),[]byte("first\nsecond\n"),0600);err!=nil{t.Fatal(err)}
 request,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionProjectFileLines,
  Path:"source.txt",Offset:2,
 })
 if err!=nil{t.Fatal(err)}
 if reply,err:=s.executeRemoteE2EEPing(context.Background(),nil,host,request);err==nil||len(reply)!=0{
  t.Fatal("compromised PHP without local TaskDeck principal got file line page")
 }
 reply,err:=s.executeRemoteE2EEPing(context.Background(),session,host,request)
 if err!=nil{t.Fatal(err)}
 value,err:=remotetransport.OpenRemoteFileLinesResult(client,reply,remoteDispatchTestID)
 if err!=nil||!strings.Contains(value,"2: second\n")||
  !strings.Contains(value,"NEXT_LINE 0\n"){
  t.Fatalf("authenticated FileLines reply failed: %v %q",err,value)
 }
 if _,err:=s.executeRemoteE2EEPing(context.Background(),session,host,request);err==nil{
  t.Fatal("replayed file page accepted")
 }
}
