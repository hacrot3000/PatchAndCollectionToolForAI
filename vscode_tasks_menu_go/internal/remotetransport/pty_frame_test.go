package remotetransport

import (
 "bytes"
 "strings"
 "testing"
)

func TestRemotePTYFrameTypedE2EERoundTripsAndNoRelayPlaintext(t *testing.T){
 id:="0123456789abcdef0123456789abcdef"
 handle:="1123456789abcdef0123456789abcdef"
 for _,tc:=range []RemotePTYFrame{
  {Handle:handle,Kind:"input",Sequence:1,Data:[]byte("echo safe\r")},
  {Handle:handle,Kind:"input",Sequence:2,Data:[]byte("xin chào")},
  {Handle:handle,Kind:"output",Sequence:3,Data:[]byte{0,0x1b,91,51,49,109,255}},
  {Handle:handle,Kind:"resize",Sequence:4,Rows:30,Cols:120},
 }{
  client,agent,_:=participants(t)
  frame,e:=SealRemotePTYFrame(client,id,tc)
  if e!=nil{t.Fatalf("canonical PTY frame rejected %+v: %v",tc,e)}
  if bytes.Contains(frame,tc.Data)&&len(tc.Data)>0{
   t.Fatal("PTY input/output became visible to the PHP relay")
  }
  decoded,e:=OpenRemotePTYFrame(agent,frame,id)
  if e!=nil||decoded.Kind!=tc.Kind||decoded.Handle!=handle||
   decoded.Sequence!=tc.Sequence||decoded.Rows!=tc.Rows||
   decoded.Cols!=tc.Cols||!bytes.Equal(decoded.Data,tc.Data){
   t.Fatalf("encrypted PTY frame corrupt: %+v %v",decoded,e)
  }
  if _,e:=OpenRemotePTYFrame(agent,frame,id);e==nil{
   t.Fatal("PTY frame E2EE ciphertext replay accepted")
  }
 }
}

func TestRemotePTYFrameRefusesUnsafeInputAndDimensions(t *testing.T){
 h:="1123456789abcdef0123456789abcdef"
 for _,v:=range []RemotePTYFrame{
  {Handle:"",Kind:"input",Sequence:1,Data:[]byte("ls")},
  {Handle:h,Kind:"input",Sequence:0,Data:[]byte("ls")},
  {Handle:h,Kind:"input",Sequence:1,Data:[]byte{}},
  {Handle:h,Kind:"input",Sequence:1,Data:bytes.Repeat([]byte("a"),RemotePTYMaxInput+1)},
  {Handle:h,Kind:"input",Sequence:1,Data:[]byte("echo ok\n")},
  {Handle:h,Kind:"input",Sequence:1,Data:[]byte("echo ok\x1b[2J")},
  {Handle:h,Kind:"input",Sequence:1,Data:[]byte{0xff,0xfe}},
  {Handle:h,Kind:"input",Sequence:1,Data:[]byte("a\rmore")},
  {Handle:h,Kind:"input",Sequence:1,Data:[]byte{0}},
  {Handle:h,Kind:"resize",Sequence:1,Rows:0,Cols:80},
  {Handle:h,Kind:"resize",Sequence:1,Rows:30,Cols:0},
  {Handle:h,Kind:"resize",Sequence:1,Rows:9,Cols:80},
  {Handle:h,Kind:"resize",Sequence:1,Rows:30,Cols:241},
  {Handle:h,Kind:"resize",Sequence:1,Rows:30,Cols:80,Data:[]byte("whoami")},
  {Handle:h,Kind:"output",Sequence:1,Data:bytes.Repeat([]byte("a"),RemotePTYMaxOutput+1)},
  {Handle:h,Kind:"output",Sequence:1,Data:[]byte{}},
  {Handle:h,Kind:"shell",Sequence:1,Data:[]byte("sh")},
 }{
  if ValidRemotePTYFrame(v){t.Fatalf("invalid PTY input accepted: %+v",v)}
  client,_,_:=participants(t)
  if encoded,e:=SealRemotePTYFrame(client,"0123456789abcdef0123456789abcdef",v);
   e==nil||len(encoded)>0{
   t.Fatalf("sealed dangerous PTY frame: %+v",v)
  }
 }
}

func TestRemotePTYFrameRejectsWrongRequestIDAndFrameType(t *testing.T){
 client,agent,_:=participants(t)
 h:="1123456789abcdef0123456789abcdef"
 id:="0123456789abcdef0123456789abcdef"
 data,e:=SealRemotePTYFrame(client,id,RemotePTYFrame{
  Handle:h,Kind:"input",Sequence:1,Data:[]byte("read only"),
 })
 if e!=nil{t.Fatal(e)}
 if _,e:=OpenRemotePTYFrame(agent,data,strings.Repeat("f",32));e==nil||agent.Established(){
  t.Fatal("wrong request id did not retire PTY encryption channel")
 }
}

func TestRemotePTYFrameUnknownFieldsNeverEstablishShellAuthority(t *testing.T){
 // This package MUST NOT register the PTY frame as RemoteAction. A valid
 // PTY framing record is not itself permission to start a shell.
 client,agent,_:=participants(t)
 id:="0123456789abcdef0123456789abcdef"
 pty,e:=SealRemotePTYFrame(client,id,RemotePTYFrame{
  Handle:"1123456789abcdef0123456789abcdef",
  Kind:"input",Sequence:1,Data:[]byte("ls"),
 })
 if e!=nil{t.Fatal(e)}
 if action,e:=OpenRemoteAction(agent,pty);e==nil||
  action.Operation!=""{
  t.Fatal("PTY data unexpectedly accepted as a TaskDeck remote action")
 }
}


func TestRemotePTYFrameRejectsExtraFieldsAndMixedDimensions(t *testing.T){
 id:="0123456789abcdef0123456789abcdef"
 h:="1123456789abcdef0123456789abcdef"
 for _,payload:=range []string{
  `{"type":"taskdeck-e2ee-bounded-pty-frame-v1","request_id":"`+id+
   `","handle":"`+h+
   `","kind":"input","sequence":1,"data":"bHM=","command":"sh","remote_control_available":false}`,
  `{"type":"taskdeck-e2ee-bounded-pty-frame-v1","request_id":"`+id+
   `","handle":"`+h+
   `","kind":"input","sequence":1,"data":"bHM=","rows":40,"remote_control_available":false}`,
  `{"type":"taskdeck-e2ee-bounded-pty-frame-v1","request_id":"`+id+
   `","handle":"`+h+
   `","kind":"resize","sequence":1,"rows":30,"cols":80,"data":"bHM=","remote_control_available":false}`,
  `{"type":"taskdeck-e2ee-bounded-pty-frame-v1","request_id":"`+id+
   `","handle":"`+h+
   `","kind":"input","sequence":1,"data":"bHM=","remote_control_available":true}`,
  `{"type":"taskdeck-e2ee-bounded-pty-frame-v1","request_id":"`+id+
   `","handle":"`+h+
   `","kind":"input","sequence":1,"sequence":2,"data":"bHM=","remote_control_available":false}`,
 }{
  client,host,_:=participants(t)
  sealed,e:=client.Seal([]byte(payload))
  if e!=nil{t.Fatal(e)}
  if frame,e:=OpenRemotePTYFrame(host,sealed,id);e==nil||
   frame.Kind!=""||host.Established(){
   t.Fatalf("malformed frame did not fail closed: %q",payload)
  }
 }
}


func TestRemotePTYFrameRejectedLocalInputDoesNotConsumeE2EEChannel(t *testing.T){
 id:="0123456789abcdef0123456789abcdef"
 handle:="1123456789abcdef0123456789abcdef"
 client,host,_:=participants(t)
 if sealed,e:=SealRemotePTYFrame(client,id,RemotePTYFrame{
  Handle:handle,Kind:"input",Sequence:1,
  Data:[]byte("echo hello\x1b[2J"),
 });e==nil||len(sealed)!=0{
  t.Fatal("dangerous PTY control input was sealed")
 }
 if !client.Established(){t.Fatal("local validation failure retired a valid E2EE channel")}
 // The next authenticated record must remain usable; do not silently
 // consume/shift the crypto sequence after a rejected local frame.
 frame,e:=SealRemotePTYFrame(client,id,RemotePTYFrame{
  Handle:handle,Kind:"resize",Sequence:1,Rows:30,Cols:120,
 })
 if e!=nil{t.Fatal(e)}
 actual,e:=OpenRemotePTYFrame(host,frame,id)
 if e!=nil||actual.Kind!="resize"||actual.Sequence!=1{
  t.Fatalf("valid PTY resize failed after rejected input: %+v %v",actual,e)
 }
}
