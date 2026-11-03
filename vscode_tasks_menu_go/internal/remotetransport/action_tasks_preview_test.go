package remotetransport

import (
 "bytes"
 "context"
 "errors"
 "strings"
 "testing"
)

func TestRemoteTasksPreviewE2EEOnlyLabelsAndReplay(t *testing.T){
 client,agent,_:=participants(t)
 request,err:=SealRemoteAction(client,RemoteAction{
  RequestID:testActionID,Operation:RemoteActionTasksPreview,
 })
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(request,[]byte("tasks.preview")) {
  t.Fatal("PHP/Relay can inspect E2EE tasks.preview operation")
 }
 action,err:=OpenRemoteAction(agent,request)
 if err!=nil||action.Operation!=RemoteActionTasksPreview {
  t.Fatalf("encrypted task action mismatch: %+v %v",action,err)
 }
 info:=RemoteTasksPreview{Total:10,Labels:[]string{"Build App","Unit tests","Deploy"}}
 ciphertext,err:=SealRemoteActionTasksPreviewResult(agent,testActionID,info)
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(ciphertext,[]byte("Build App")) {
  t.Fatal("PHP/Relay observed task labels")
 }
 opened,err:=OpenRemoteActionTasksPreviewResult(client,ciphertext,testActionID)
 if err!=nil||opened.Total!=info.Total||len(opened.Labels)!=3 ||
  opened.Labels[0]!="Build App" {
  t.Fatalf("decoded task preview invalid %+v %v",opened,err)
 }
 if _,err:=OpenRemoteActionTasksPreviewResult(client,ciphertext,testActionID);err==nil{
  t.Fatal("replayed task label response was accepted")
 }
 if client.Established(){t.Fatal("replay failed to poison AEAD session")}
}

func TestRemoteTasksPreviewRejectsMaliciousReplyFields(t *testing.T){
 valid:=`{"type":"taskdeck-e2ee-tasks-preview-result-v1","request_id":"0123456789abcdef0123456789abcdef","total":1,"labels":["Build"],"remote_control_available":false`
 cases:=map[string]string{
  "unknown command":valid+`,"command":"rm -rf /"}`,
  "unknown script":valid+`,"args":["--unsafe"]}`,
  "remote control true":strings.Replace(valid+`}`,`"remote_control_available":false`,`"remote_control_available":true`,1),
  "duplicate total":valid+`,"total":100}`,
  "wrong ID":strings.Replace(valid+`}`,testActionID,strings.Repeat("a",32),1),
  "injected HTML control":strings.Replace(valid+`}`,"Build",`Build\nShell`,1),
  "too many labels":strings.Replace(valid+`}`,`["Build"]`,`["one","two","three","four"]`,1),
  "negative count":strings.Replace(valid+`}`,`"total":1`,`"total":-1`,1),
  "null list":strings.Replace(valid+`}`,`["Build"]`,`null`,1),
  "path field":valid+`,"workspace":"/root/private"}`,
 }
 for name,clear:=range cases {
  t.Run(name,func(t *testing.T){
   client,agent,_:=participants(t)
   frame,err:=agent.Seal([]byte(clear))
   if err!=nil{t.Fatal(err)}
   if _,err:=OpenRemoteActionTasksPreviewResult(client,frame,testActionID);
    !errors.Is(err,ErrInvalidRemoteAction) {
    t.Fatalf("malicious preview reply accepted: %s (%v)",name,err)
   }
   if client.Established(){t.Fatal("forged task preview did not poison authenticated channel")}
  })
 }
}

func TestRemoteTasksPreviewRejectsUnsafeHostLabels(t *testing.T){
 client,_,_:=participants(t)
 for _,label:=range []string{"",strings.Repeat("x",65),"command\nshutdown","left\u202eright"}{
  if _,err:=SealRemoteActionTasksPreviewResult(client,testActionID,
   RemoteTasksPreview{Total:1,Labels:[]string{label}});err==nil {
   t.Fatalf("unsafe task preview label admitted: %q",label)
  }
 }
 if _,err:=SealRemoteAction(client,RemoteAction{
  RequestID:testActionID,Operation:"tasks.run",
 });err==nil{t.Fatal("TaskDeck task execution smuggled into read-only transport")}
}

func TestRemoteTasksPreviewNativeClientTypedExchange(t *testing.T){
 client,agent,_:=participants(t)
 exchange:=pingExchangeFunc(func(ctx context.Context,frame []byte)([]byte,error){
  request,err:=OpenRemoteAction(agent,frame)
  if err!=nil||request.Operation!=RemoteActionTasksPreview{
   return nil,ErrRemoteActionDenied
  }
  return SealRemoteActionTasksPreviewResult(agent,request.RequestID,
   RemoteTasksPreview{Total:2,Labels:[]string{"Verify","Build"}})
 })
 native,err:=NewPingClient(client,exchange)
 if err!=nil{t.Fatal(err)}
 defer native.Close()
 preview,err:=native.TasksPreview(context.Background())
 if err!=nil||preview.Total!=2||len(preview.Labels)!=2||preview.Labels[0]!="Verify" {
  t.Fatalf("native AEAD task preview failed: %+v %v",preview,err)
 }
}
