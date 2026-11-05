package remotetransport

import (
 "context"
 "errors"
 "strings"
 "testing"
)

func TestRemoteTerminalNativeClientFixedE2EEOperations(t *testing.T){
 client,agent,_:=participants(t)
 count:=0
 exchange:=pingExchangeFunc(func(ctx context.Context,frame []byte)([]byte,error){
  request,err:=OpenRemoteAction(agent,frame)
  if err!=nil{return nil,err}
  count++
  switch count{
  case 1:
   if request.Operation!=RemoteActionTerminalOpen{return nil,ErrInvalidRemoteAction}
  case 2:
   if request.Operation!=RemoteActionTerminalWrite||request.TerminalInput!="echo test\n"{
    return nil,ErrInvalidRemoteAction
   }
  case 3:
   if request.Operation!=RemoteActionTerminalResize||request.Cols!=120||request.Rows!=32{
    return nil,ErrInvalidRemoteAction
   }
  case 4:
   if request.Operation!=RemoteActionTerminalPoll{return nil,ErrInvalidRemoteAction}
  case 5:
   if request.Operation!=RemoteActionTerminalClose{return nil,ErrInvalidRemoteAction}
  default:return nil,ErrInvalidRemoteAction
  }
  progress:=RemoteTerminalProgress{State:"running"}
  if count==4{progress.Output="test\n"}
  if count==5{progress.State="closed"}
  return SealRemoteTerminalProgress(agent,request.RequestID,progress)
 })
 native,err:=NewPingClient(client,exchange)
 if err!=nil{t.Fatal(err)}
 defer native.Close()
 if p,err:=native.TerminalOpen(context.Background());err!=nil||p.State!="running"{t.Fatalf("open %+v %v",p,err)}
 if p,err:=native.TerminalWrite(context.Background(),"echo test\n");err!=nil||p.State!="running"{t.Fatalf("write %+v %v",p,err)}
 if p,err:=native.TerminalResize(context.Background(),120,32);err!=nil||p.State!="running"{t.Fatalf("resize %+v %v",p,err)}
 if p,err:=native.TerminalPoll(context.Background());err!=nil||p.Output!="test\n"{t.Fatalf("poll %+v %v",p,err)}
 if p,err:=native.TerminalClose(context.Background());err!=nil||p.State!="closed"{t.Fatalf("close %+v %v",p,err)}
 if count!=5{t.Fatalf("wrong number of E2EE frames: %d",count)}
}

func TestRemoteTerminalInputAndDimensionsStrict(t *testing.T){
 for _,value:=range []string{"ls\n","Sửa lỗi\n",string([]byte{3}),string([]byte{4}),"x\b"}{
  if !ValidRemoteTerminalInput(value){t.Fatalf("valid short PTY input denied: %q",value)}
 }
 for _,value:=range []string{"",strings.Repeat("a",97),"unsafe\x1b[2J",
  "nul\x00","unicode\u200bcontrol",string([]byte{0xff})}{
  if ValidRemoteTerminalInput(value){t.Fatalf("invalid PTY input admitted: %q",value)}
 }
 for _,s:=range [][2]int{{0,24},{80,0},{19,24},{181,24},{80,7},{80,61}}{
  if ValidRemoteTerminalSize(s[0],s[1]){t.Fatalf("invalid PTY size: %+v",s)}
 }
 for _,s:=range [][2]int{{80,24},{120,32},{180,60}}{
  if !ValidRemoteTerminalSize(s[0],s[1]){t.Fatalf("valid PTY size denied: %+v",s)}
 }
}

func TestRemoteTerminalSchemaRejectsUnknownArgsAndForgedResults(t *testing.T){
 const prefix=`{"type":"taskdeck-e2ee-action-v1","request_id":"0123456789abcdef0123456789abcdef",`
 for name,raw:=range map[string]string{
  "open with shell":prefix+`"operation":"terminal.open","command":"/bin/sh"}`,
  "write without input":prefix+`"operation":"terminal.write"}`,
  "write with task id":prefix+`"operation":"terminal.write","terminal_input":"ok","task_id":2}`,
  "write with extra key":prefix+`"operation":"terminal.write","terminal_input":"ok","shell":true}`,
  "resize missing row":prefix+`"operation":"terminal.resize","cols":80}`,
  "resize null":prefix+`"operation":"terminal.resize","cols":null,"rows":24}`,
  "poll with terminal ID":prefix+`"operation":"terminal.poll","terminal_id":1}`,
  "duplicate input":prefix+`"operation":"terminal.write","terminal_input":"ok","terminal_input":"unsafe"}`,
  "unreviewed shell":prefix+`"operation":"terminal.exec"}`,
 }{
  t.Run(name,func(t *testing.T){
   client,agent,_:=participants(t)
   frame,err:=client.Seal([]byte(raw))
   if err!=nil{t.Fatal(err)}
   if _,err:=OpenRemoteAction(agent,frame);!errors.Is(err,ErrInvalidRemoteAction){
    t.Fatalf("forged PTY schema admitted: %v",err)
   }
   if agent.Established(){t.Fatal("bad authenticated PTY request left channel established")}
  })
 }
 good:=RemoteTerminalProgress{State:"running",Output:"test\n"}
 client,agent,_:=participants(t)
 frame,err:=SealRemoteTerminalProgress(agent,testActionID,good)
 if err!=nil{t.Fatal(err)}
 p,err:=OpenRemoteTerminalProgress(client,frame,testActionID)
 if err!=nil||p!=good{t.Fatalf("valid PTY result lost: %+v %v",p,err)}
 for _,bad:=range []RemoteTerminalProgress{
  {State:"running",Output:"bad\x1b[31m"},
  {State:"closed",Output:"late"},
  {State:"closed",More:true},
  {State:"missing"},
  {State:"running",Output:strings.Repeat("x",129)},
 }{
  _,agent,_:=participants(t)
  if _,err:=SealRemoteTerminalProgress(agent,testActionID,bad);!errors.Is(err,ErrInvalidRemoteAction){
   t.Fatalf("malformed PTY result sealed: %+v %v",bad,err)
  }
 }
}
