package remotetransport

import (
 "bytes"
 "context"
 "errors"
 "fmt"
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
  "too many labels":strings.Replace(strings.Replace(valid+`}`,`"total":1`,`"total":25`,1),
    `["Build"]`,`["one","two","three","four","five","six","seven","eight","nine","10","11","12","13","14","15","16","17","18","19","20","21","22","23","24","25"]`,1),
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

func TestRemoteTasksPreviewAdaptiveFitNeverExpandsRelayPayload(t *testing.T){
 client,agent,_:=participants(t)
 // Go JSON encoding escapes '<' as \u003c, so eight valid labels
 // may be too large for the fixed authenticated application frame.
 large:=strings.Repeat("<",MaxRemoteTaskLabelBytes)
 labels:=make([]string,MaxRemoteTasksPreviewLabels)
 for i:=range labels{labels[i]=large}
 full:=RemoteTasksPreview{Total:MaxRemoteTasksCount,Labels:labels}
 trimmed,err:=fitRemoteTasksPreview(full,testActionID)
 if err!=nil || len(trimmed.Labels)>=len(full.Labels){
  t.Fatalf("bounded payload was not shrunk: %d %v",len(trimmed.Labels),err)
 }
 if full.Total!=trimmed.Total || len(full.Labels)!=MaxRemoteTasksPreviewLabels {
  t.Fatal("adaptive serialization mutated caller state or task count")
 }
 frame,err:=SealRemoteActionTasksPreviewResult(agent,testActionID,full)
 if err!=nil{t.Fatalf("valid display labels caused failed E2EE read: %v",err)}
 if len(frame)>maxRemoteActionPlaintext+64{
  t.Fatalf("task labels exceeded fixed E2EE mailbox: %d",len(frame))
 }
 decoded,err:=OpenRemoteActionTasksPreviewResult(client,frame,testActionID)
 if err!=nil || decoded.Total!=full.Total ||
  len(decoded.Labels)!=len(trimmed.Labels) {
  t.Fatalf("bounded label preview failed E2EE roundtrip: %+v %v",decoded,err)
 }
 for _,label:=range decoded.Labels{
  if label!=large {t.Fatal("adaptive truncation altered a task label")}
 }
}

func TestRemoteTasksPreviewEightShortLabelsFitInOneEncryptedAction(t *testing.T){
 client,agent,_:=participants(t)
 labels:=[]string{"Task A","Task B","Task C","Task D","Task E","Task F","Task G","Task H"}
 preview:=RemoteTasksPreview{Total:100,Labels:labels}
 frame,err:=SealRemoteActionTasksPreviewResult(agent,testActionID,preview)
 if err!=nil{t.Fatal(err)}
 opened,err:=OpenRemoteActionTasksPreviewResult(client,frame,testActionID)
 if err!=nil||len(opened.Labels)!=len(labels)||opened.Total!=100 {
  t.Fatalf("short task menu was truncated unnecessarily: %+v %v",opened,err)
 }
}

func TestRemoteTasksPreviewTwentyFourShortLabelsFitOneEncryptedAction(t *testing.T){
 client,agent,_:=participants(t)
 labels:=make([]string,0,MaxRemoteTasksPreviewLabels)
 for i:=0;i<MaxRemoteTasksPreviewLabels;i++{
  labels=append(labels,fmt.Sprintf("T%02d",i))
 }
 frame,err:=SealRemoteActionTasksPreviewResult(agent,testActionID,
  RemoteTasksPreview{Total:120,Labels:labels})
 if err!=nil{t.Fatal(err)}
 info,err:=OpenRemoteActionTasksPreviewResult(client,frame,testActionID)
 if err!=nil || len(info.Labels)!=MaxRemoteTasksPreviewLabels || info.Total!=120 {
  t.Fatalf("24 short labels did not fit existing AEAD envelope: %+v %v",info,err)
 }
}

func TestRemoteTasksPreviewNextNativeClientNoArgumentPaging(t *testing.T){
 client,agent,_:=participants(t)
 index:=0
 exchange:=pingExchangeFunc(func(ctx context.Context,frame []byte)([]byte,error){
  action,err:=OpenRemoteAction(agent,frame)
  if err!=nil{return nil,err}
  index++
  expected:=RemoteActionTasksPreview
  if index==2 {expected=RemoteActionTasksPreviewNext}
  if action.Operation!=expected{return nil,ErrInvalidRemoteAction}
  labels:=[]string{"page one"}
  if index==2{labels=[]string{"page two"}}
  return SealRemoteActionTasksPreviewResult(agent,action.RequestID,
   RemoteTasksPreview{Total:2,Labels:labels})
 })
 native,err:=NewPingClient(client,exchange)
 if err!=nil{t.Fatal(err)}
 defer native.Close()
 first,err:=native.TasksPreview(context.Background())
 if err!=nil||len(first.Labels)!=1||first.Labels[0]!="page one"{
  t.Fatalf("initial E2EE read invalid: %+v %v",first,err)
 }
 next,err:=native.TasksPreviewNext(context.Background())
 if err!=nil||len(next.Labels)!=1||next.Labels[0]!="page two"{
  t.Fatalf("host-owned next-page read invalid: %+v %v",next,err)
 }
 if index!=2 {t.Fatal("unexpected Relay request count")}
}

func TestRemoteTasksNextRejectsUntrustedClientChosenOffset(t *testing.T){
 client,agent,_:=participants(t)
 request,err:=SealRemoteAction(client,RemoteAction{
  RequestID:testActionID,Operation:RemoteActionTasksPreviewNext,
 })
 if err!=nil{t.Fatal(err)}
 if bytes.Contains(request,[]byte("tasks.preview.next")){
  t.Fatal("compromised PHP can read task pagination requests")
 }
 got,err:=OpenRemoteAction(agent,request)
 if err!=nil||got.Operation!=RemoteActionTasksPreviewNext{
  t.Fatalf("fixed next-page operation not authenticated: %+v %v",got,err)
 }
 // Any extra JSON field permanently poisons the authenticated channel.
 client2,agent2,_:=participants(t)
 forged,err:=client2.Seal([]byte(
  "{\"type\":\"taskdeck-e2ee-action-v1\",\"request_id\":\""+
   testActionID+"\",\"operation\":\"tasks.preview.next\",\"offset\":999999}"))
 if err!=nil{t.Fatal(err)}
 if _,err:=OpenRemoteAction(agent2,forged);err==nil||agent2.Established(){
  t.Fatal("client-supplied task offset bypassed exact-key E2EE contract")
 }
}
