package remotetransport

import (
 "context"
 "encoding/json"
 "errors"
 "strings"
 "testing"
)

func TestRemoteFilesReadStrictTypedE2EE(t *testing.T){
 c,a,_:=participants(t)
 input:=RemoteFilesReadPage{Page:1,Name:"notes.txt",Text:"Hello\nWorld",More:true}
 frame,err:=SealRemoteFilesReadPage(a,testActionID,input)
 if err!=nil{t.Fatal(err)}
 result,err:=OpenRemoteFilesReadPage(c,frame,testActionID)
 if err!=nil||result!=input{t.Fatalf("read page corrupted: %+v %v",result,err)}
 for _,bad:=range []RemoteFilesReadPage{
  {Page:0,Name:"notes.txt",Text:"hello"},
  {Page:1,Name:".env",Text:"secret"},
  {Page:1,Name:"../file",Text:"secret"},
  {Page:1,Name:"a/b",Text:"secret"},
  {Page:1,Name:"valid.txt",Text:"hello\rworld"},
  {Page:1,Name:"valid.txt",Text:"hidden\x00data"},
  {Page:1,Name:"valid.txt",Text:"secret",More:false},
  {Page:1,Name:"valid.txt",Text:"",More:true},
  {Page:2,Name:"valid.txt",Text:"",More:false},
  {Page:17,Name:"valid.txt",Text:"content"},
  {Page:1,Name:"valid.txt",Text:strings.Repeat("a",221)},
 }{
  if bad.Page==1&&bad.Name=="valid.txt"&&bad.Text=="secret"&&
   !bad.More{continue} // valid terminal page
  c,_,_:=participants(t)
  if _,err:=SealRemoteFilesReadPage(c,testActionID,bad);
   !errors.Is(err,ErrInvalidRemoteAction){
   t.Fatalf("accepted invalid page %+v: %v",bad,err)
  }
 }
 raw,err:=json.Marshal(remoteFilesReadPageWire{
  Type:RemoteFilesReadPageProtocol,RequestID:testActionID,
  Page:1,Name:"notes.txt",Text:"safe",More:false,
  RemoteControlAvailable:false,
 })
 if err!=nil{t.Fatal(err)}
 for name,body:=range map[string]string{
  "duplicate":strings.Replace(string(raw),"}",`,"name":"secrets.txt"}`,1),
  "unknown":strings.Replace(string(raw),"}",`,"path":"../../secret"}`,1),
  "remote-control":strings.Replace(string(raw),`"remote_control_available":false`,`"remote_control_available":true`,1),
  "invalid-nul":strings.Replace(string(raw),`"text":"safe"`,`"text":"\u0000"`,1),
 }{
  t.Run(name,func(t *testing.T){
   c,a,_:=participants(t)
   frame,err:=a.Seal([]byte(body))
   if err!=nil{t.Fatal(err)}
   if _,err:=OpenRemoteFilesReadPage(c,frame,testActionID);
    !errors.Is(err,ErrInvalidRemoteAction){t.Fatalf("forged content accepted: %v",err)}
   if c.Established(){t.Fatal("invalid content failed to close E2EE channel")}
  })
 }
}
func TestRemoteFilesReadBoundedUtf8PagesAndMailbox(t *testing.T){
 content:=strings.Repeat("Xin chào!\n",80)
 if len(content)>MaxRemoteFilesReadSourceBytes{t.Fatal("test content is too large")}
 var whole strings.Builder
 start,page:=0,1
 for start<len(content){
  got,next,err:=BoundRemoteFilesReadPage(testActionID,"notes.txt",content,start,page)
  if err!=nil||!ValidRemoteFilesReadPage(got)||next<=start{
   t.Fatalf("invalid page %d: %+v %v",page,got,err)
  }
  c,a,_:=participants(t)
  frame,err:=SealRemoteFilesReadPage(a,testActionID,got)
  if err!=nil{t.Fatal(err)}
  roundTrip,err:=OpenRemoteFilesReadPage(c,frame,testActionID)
  if err!=nil||roundTrip!=got{t.Fatalf("E2EE mismatch: %+v %v",roundTrip,err)}
  whole.WriteString(got.Text)
  start=next;page++
  if page>16{t.Fatal("too many pages for 2-KiB bound")}
 }
 if whole.String()!=content{t.Fatal("UTF-8 byte-loss across read pages")}
 if _,_,err:=BoundRemoteFilesReadPage(testActionID,"notes.txt",strings.Repeat("x",2049),0,1);
  !errors.Is(err,ErrInvalidRemoteAction){t.Fatal("oversized file was partially disclosed")}
 if _,_,err:=BoundRemoteFilesReadPage(testActionID,"notes.txt","safe\n"+"unsafe\r",0,1);
  !errors.Is(err,ErrInvalidRemoteAction){t.Fatal("unsafe later content was partially disclosed")}
}

func TestRemoteFilesReadNativeFixedNoFilename(t *testing.T){
 for _,op:=range []string{RemoteActionFilesReadStart,RemoteActionFilesReadNext}{
  t.Run(op,func(t *testing.T){
   c,a,_:=participants(t)
   exchange:=pingExchangeFunc(func(ctx context.Context,frame []byte)([]byte,error){
    request,err:=OpenRemoteAction(a,frame)
    if err!=nil||request.Operation!=op||request.Text!=""||
     request.Message!=""||request.TaskID!=0||request.Choice!=0||
     request.Snapshot!=""||request.FileIndex!=0||
     request.TerminalInput!=""||request.Cols!=0||request.Rows!=0{
     return nil,ErrInvalidRemoteAction
    }
    return SealRemoteFilesReadPage(a,request.RequestID,RemoteFilesReadPage{
     Page:1,Name:"notes.txt",Text:"safe",More:false,
    })
   })
   native,err:=NewPingClient(c,exchange)
   if err!=nil{t.Fatal(err)}
   defer native.Close()
   var got RemoteFilesReadPage
   if op==RemoteActionFilesReadStart{
    got,err=native.FilesReadStart(context.Background())
   }else{got,err=native.FilesReadNext(context.Background())}
   if err!=nil||got.Name!="notes.txt"{t.Fatalf("native result: %+v %v",got,err)}
   for _,value:=range []RemoteAction{
    {RequestID:testActionID,Operation:op,Text:"notes.txt"},
    {RequestID:testActionID,Operation:op,Message:"../../etc/passwd"},
    {RequestID:testActionID,Operation:op,FileIndex:1},
   }{
    client,_,_:=participants(t)
    if _,err:=SealRemoteAction(client,value);!errors.Is(err,ErrInvalidRemoteAction){
     t.Fatalf("client path/offset accepted %+v",value)
    }
   }
  })
 }
}
