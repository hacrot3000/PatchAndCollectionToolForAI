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

func largeLineFixture(t *testing.T,root string)([]byte,string){
 t.Helper()
 var out strings.Builder
 for i:=0;i<600;i++{
  out.WriteString("source unchanged CRLF\r\n")
 }
 data:=[]byte(out.String())
 if len(data)<=4096{t.Fatal("fixture must exceed 4KiB")}
 name:=filepath.Join(root,"source.txt")
 if err:=os.WriteFile(name,data,0600);err!=nil{t.Fatal(err)}
 sum:=sha256.Sum256(data)
 return data,hex.EncodeToString(sum[:])
}

func TestLargeLineEditMaintainsAllOtherBytesAndCRLF(t *testing.T){
 s,session,client,host:=remoteDispatchFixtureWithRole(t,remotetransport.RoleAdmin)
 original,hash:=largeLineFixture(t,s.Workspace)
 t.Setenv("TASKDECK_REMOTE_FILE_REPLACE","1")
 t.Setenv("TASKDECK_REMOTE_TEXT_EDIT","1")
 t.Setenv("TASKDECK_REMOTE_LARGE_LINE_EDIT","1")
 a:=remotetransport.RemoteAction{RequestID:remoteDispatchTestID,
  Operation:remotetransport.RemoteActionProjectFileEditLine,
  Path:"source.txt",Offset:300,Content:"new source ✓",ExpectedSHA256:hash}
 req,err:=remotetransport.SealRemoteAction(client,a)
 if err!=nil{t.Fatal(err)}
 reply,err:=s.executeRemoteE2EEPing(context.Background(),session,host,req)
 if err!=nil{t.Fatal(err)}
 sha,err:=remotetransport.OpenRemoteLargeLineEditResult(client,reply,remoteDispatchTestID)
 if err!=nil{t.Fatal(err)}
 wanted:=bytes.Replace(original,[]byte("source unchanged CRLF\r\n"),
  []byte("new source ✓\r\n"),1)
 // Must replace exactly line 300, not the first matching occurrence.
 expected,ok:=remoteReplaceExistingSourceLine(original,300,"new source ✓")
 if !ok||bytes.Equal(wanted,expected){t.Fatal("fixture failed to distinguish edited lines")}
 latest,err:=os.ReadFile(filepath.Join(s.Workspace,"source.txt"))
 if err!=nil||!bytes.Equal(latest,expected){t.Fatal("non-target bytes or CRLF changed")}
 sum:=sha256.Sum256(expected)
 if sha!=hex.EncodeToString(sum[:]){t.Fatal("receipt is not post-write file digest")}
 if _,err:=s.executeRemoteE2EEPing(context.Background(),session,host,req);err==nil{
  t.Fatal("E2EE replay accepted")
 }
}

func TestLargeLineEditDeniesUnsafeCASAndNoOp(t *testing.T){
 for _,kind:=range []string{"wrong-sha","missing-line","noop","symlink","too-large","binary"}{
  t.Run(kind,func(t *testing.T){
   s,session,client,host:=remoteDispatchFixtureWithRole(t,remotetransport.RoleAdmin)
   original,hash:=largeLineFixture(t,s.Workspace)
   name:=filepath.Join(s.Workspace,"source.txt")
   line:=int64(300)
   replacement:="new source"
   switch kind{
   case "wrong-sha":hash=strings.Repeat("f",64)
   case "missing-line":line=1000
   case "noop":replacement="source unchanged CRLF"
   case "symlink":
    if err:=os.Remove(name);err!=nil{t.Fatal(err)}
    if err:=os.Symlink(filepath.Join(t.TempDir(),"elsewhere"),name);err!=nil{t.Fatal(err)}
   case "too-large":
    big:=[]byte(strings.Repeat("x",2<<20+1))
    if err:=os.WriteFile(name,big,0600);err!=nil{t.Fatal(err)}
   case "binary":
    if err:=os.WriteFile(name,[]byte{0xff,0xfe,0},0600);err!=nil{t.Fatal(err)}
   }
   t.Setenv("TASKDECK_REMOTE_FILE_REPLACE","1")
   t.Setenv("TASKDECK_REMOTE_TEXT_EDIT","1")
   t.Setenv("TASKDECK_REMOTE_LARGE_LINE_EDIT","1")
   req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
    RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionProjectFileEditLine,
    Path:"source.txt",Offset:line,Content:replacement,ExpectedSHA256:hash})
   if err!=nil{t.Fatal(err)}
   reply,err:=s.executeRemoteE2EEPing(context.Background(),session,host,req)
   if err==nil||len(reply)!=0{t.Fatal("invalid CAS or content accepted")}
   if kind=="wrong-sha"||kind=="missing-line"||kind=="noop"{
    latest,err:=os.ReadFile(name)
    if err!=nil||!bytes.Equal(latest,original){t.Fatal("rejected action changed file")}
   }
  })
 }
}

func TestLargeLineEditRequiresThreeHostFlagsAndRealPrincipal(t *testing.T){
 for _,kind:=range []string{"flag-file","flag-text","flag-large","viewer","php-only","admin"}{
  t.Run(kind,func(t *testing.T){
   role:=remotetransport.RoleAdmin
   if kind=="viewer"{role=remotetransport.RoleViewer}
   s,session,client,host:=remoteDispatchFixtureWithRole(t,role)
   original,hash:=largeLineFixture(t,s.Workspace)
   t.Setenv("TASKDECK_REMOTE_FILE_REPLACE","1")
   t.Setenv("TASKDECK_REMOTE_TEXT_EDIT","1")
   t.Setenv("TASKDECK_REMOTE_LARGE_LINE_EDIT","1")
   switch kind{
   case "flag-file":t.Setenv("TASKDECK_REMOTE_FILE_REPLACE","")
   case "flag-text":t.Setenv("TASKDECK_REMOTE_TEXT_EDIT","")
   case "flag-large":t.Setenv("TASKDECK_REMOTE_LARGE_LINE_EDIT","")
   case "php-only":session=nil
   }
   packet,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
    RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionProjectFileEditLine,
    Path:"source.txt",Offset:2,Content:"replacement",ExpectedSHA256:hash})
   if err!=nil{t.Fatal(err)}
   reply,err:=s.executeRemoteE2EEPing(context.Background(),session,host,packet)
   if kind=="admin"{
    if err!=nil{t.Fatal(err)}
    if _,err:=remotetransport.OpenRemoteLargeLineEditResult(client,reply,remoteDispatchTestID);err!=nil{t.Fatal(err)}
   }else{
    if err==nil||len(reply)>0{t.Fatalf("%s bypassed host security",kind)}
    latest,err:=os.ReadFile(filepath.Join(s.Workspace,"source.txt"))
    if err!=nil||!bytes.Equal(latest,original){t.Fatal("unauthorized client edited source")}
   }
  })
 }
}

func TestLargeLineDeletePreservesUntouchedBytesAndReceipts(t *testing.T){
 s,session,client,host:=remoteDispatchFixtureWithRole(t,remotetransport.RoleAdmin)
 original,hash:=largeLineFixture(t,s.Workspace)
 t.Setenv("TASKDECK_REMOTE_FILE_REPLACE","1")
 t.Setenv("TASKDECK_REMOTE_TEXT_EDIT","1")
 t.Setenv("TASKDECK_REMOTE_LARGE_LINE_EDIT","1")
 req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
  RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionProjectFileDeleteLine,
  Path:"source.txt",Offset:300,ExpectedSHA256:hash})
 if err!=nil{t.Fatal(err)}
 reply,err:=s.executeRemoteE2EEPing(context.Background(),session,host,req)
 if err!=nil{t.Fatal(err)}
 written,err:=remotetransport.OpenRemoteLargeLineDeleteResult(client,reply,remoteDispatchTestID)
 if err!=nil{t.Fatal(err)}
 expected,ok:=remoteDeleteExistingSourceLine(original,300)
 if !ok{t.Fatal("delete fixture denied")}
 data,err:=os.ReadFile(filepath.Join(s.Workspace,"source.txt"))
 if err!=nil||!bytes.Equal(data,expected){t.Fatal("non-target CRLF bytes changed")}
 if len(data)!=len(original)-len("source unchanged CRLF\r\n"){t.Fatal("wrong number of source bytes removed")}
 sum:=sha256.Sum256(expected)
 if written!=hex.EncodeToString(sum[:]){t.Fatal("receipt did not match new file")}
 if _,err:=s.executeRemoteE2EEPing(context.Background(),session,host,req);err==nil{
  t.Fatal("accepted E2EE replay")
 }
}

func TestLargeLineDeleteRejectsUnsafeSourceAndMissingAuthorization(t *testing.T){
 for _,kind:=range []string{"stale-sha","missing-line","single-line","symlink","oversized","viewer","no-flag","no-principal"}{
  t.Run(kind,func(t *testing.T){
   role:=remotetransport.RoleAdmin
   if kind=="viewer"{role=remotetransport.RoleViewer}
   s,session,client,host:=remoteDispatchFixtureWithRole(t,role)
   before,sha:=largeLineFixture(t,s.Workspace)
   path:=filepath.Join(s.Workspace,"source.txt")
   offset:=int64(300)
   t.Setenv("TASKDECK_REMOTE_FILE_REPLACE","1")
   t.Setenv("TASKDECK_REMOTE_TEXT_EDIT","1")
   t.Setenv("TASKDECK_REMOTE_LARGE_LINE_EDIT","1")
   switch kind{
   case "stale-sha":sha=strings.Repeat("f",64)
   case "missing-line":offset=100000
   case "single-line":if err:=os.WriteFile(path,[]byte(strings.Repeat("x",4000)),0600);err!=nil{t.Fatal(err)}
   case "symlink":
    if err:=os.Remove(path);err!=nil{t.Fatal(err)}
    if err:=os.Symlink(filepath.Join(t.TempDir(),"escape"),path);err!=nil{t.Fatal(err)}
   case "oversized":if err:=os.WriteFile(path,[]byte(strings.Repeat("x",2<<20+1)),0600);err!=nil{t.Fatal(err)}
   case "no-flag":t.Setenv("TASKDECK_REMOTE_LARGE_LINE_EDIT","")
   case "no-principal":session=nil
   }
   req,err:=remotetransport.SealRemoteAction(client,remotetransport.RemoteAction{
    RequestID:remoteDispatchTestID,Operation:remotetransport.RemoteActionProjectFileDeleteLine,
    Path:"source.txt",Offset:offset,ExpectedSHA256:sha})
   if err!=nil{t.Fatal(err)}
   reply,err:=s.executeRemoteE2EEPing(context.Background(),session,host,req)
   if err==nil||len(reply)!=0{t.Fatalf("bypass: %s",kind)}
   if kind=="stale-sha"||kind=="missing-line"||kind=="viewer"||kind=="no-flag"||kind=="no-principal"{
    after,err:=os.ReadFile(path)
    if err!=nil||!bytes.Equal(after,before){t.Fatal("denied delete mutated source")}
   }
  })
 }
}

func TestLargeLineDeleteByteSemantics(t *testing.T){
 for _,tc:=range []struct{before string;line int64;want string}{
  {"a\r\nb\r\nc",2,"a\r\nc"},
  {"a\nb",2,"a"},
  {"a\r\nb",2,"a"},
  {"a\nb\r\nc",3,"a\nb"},
  {"a\nb\n",1,"b\n"},
  {"single",1,""},
  {"\n",1,""},
  {"a\nb\nc",3,"a\nb"},
 }{
  got,ok:=remoteDeleteExistingSourceLine([]byte(tc.before),tc.line)
  if !ok||string(got)!=tc.want{t.Fatalf("delete line %d: %q want %q, valid=%v",tc.line,got,tc.want,ok)}
 }
 if _,ok:=remoteDeleteExistingSourceLine([]byte("a\n"),2);ok{t.Fatal("accepted phantom EOF line")}
 if _,ok:=remoteDeleteExistingSourceLine([]byte("x\x00y"),1);ok{t.Fatal("accepted binary file")}
}
