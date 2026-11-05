package remotetransport

import (
 "encoding/json"
 "unicode"
 "unicode/utf8"
)

// PTY framing is deliberately NOT a mounted Remote action. The PHP Relay
// must never gain shell/PTY authority from the presence of this codec.
// A future host adapter must first supply TerminalCreate/ControlOwn RBAC,
// local host opt-in, an owned PTY session, one-use approval and strict
// per-session input/output quotas before it exposes this encrypted type.
const remotePTYFrameType="taskdeck-e2ee-bounded-pty-frame-v1"
const RemotePTYMaxInput=256
const RemotePTYMaxOutput=2048
const RemotePTYMinRows=10
const RemotePTYMaxRows=120
const RemotePTYMinCols=20
const RemotePTYMaxCols=240

type RemotePTYFrame struct{
 Handle string
 Kind string // "input", "output" or "resize"
 Sequence uint64
 Data []byte
 Rows uint16
 Cols uint16
}

type remotePTYFrameWire struct{
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 Handle string `json:"handle"`
 Kind string `json:"kind"`
 Sequence uint64 `json:"sequence"`
 Data []byte `json:"data,omitempty"`
 Rows uint16 `json:"rows,omitempty"`
 Cols uint16 `json:"cols,omitempty"`
 RemoteControlAvailable bool `json:"remote_control_available"`
}

func ValidRemotePTYFrame(v RemotePTYFrame)bool{
 if !ValidManagedTaskHandle(v.Handle)||v.Sequence==0{return false}
 switch v.Kind{
 case "input":
  // No arbitrary binary/escape-input injection through this first typed
  // frame. Only bounded printable UTF-8 and a SINGLE optional final CR are
  // allowed. Even this type is not authorized to launch a command.
  if len(v.Data)<1||len(v.Data)>RemotePTYMaxInput||
   !utf8.Valid(v.Data)||v.Rows!=0||v.Cols!=0{return false}
  for offset,r:=range string(v.Data){
   if r=='\r'&&offset==len(v.Data)-1{continue}
   // A valid UTF-8 string can still contain bidi controls, zero-width
   // format characters and Unicode line separators that spoof a shell
   // preview. Permit visible printable characters and ASCII space only.
   if !unicode.IsPrint(r)||(unicode.IsSpace(r)&&r!=' '){return false}
  }
  return true
 case "output":
  return len(v.Data)>0&&len(v.Data)<=RemotePTYMaxOutput&&
   v.Rows==0&&v.Cols==0
 case "resize":
  return len(v.Data)==0&&v.Rows>=RemotePTYMinRows&&
   v.Rows<=RemotePTYMaxRows&&v.Cols>=RemotePTYMinCols&&
   v.Cols<=RemotePTYMaxCols
 default:return false
 }
}

func SealRemotePTYFrame(ch *Channel,id string,v RemotePTYFrame)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||
  !ValidRemotePTYFrame(v){return nil,ErrInvalidRemoteAction}
 data,err:=json.Marshal(remotePTYFrameWire{
  Type:remotePTYFrameType,RequestID:id,
  Handle:v.Handle,Kind:v.Kind,Sequence:v.Sequence,
  Data:v.Data,Rows:v.Rows,Cols:v.Cols,
  RemoteControlAvailable:false,
 })
 if err!=nil||len(data)>4096{return nil,ErrInvalidRemoteAction}
 defer wipeLoginBytes(data)
 return ch.Seal(data)
}

func OpenRemotePTYFrame(ch *Channel,ciphertext []byte,id string)(RemotePTYFrame,error){
 denied:=ErrInvalidRemoteAction
 if ch==nil||!ch.Established()||!validActionID(id){return RemotePTYFrame{},denied}
 data,err:=ch.Open(ciphertext)
 if err!=nil{return RemotePTYFrame{},denied}
 defer wipeLoginBytes(data)
 base:=[]string{"type","request_id","handle","kind","sequence","remote_control_available"}
 // Distinct canonical layouts for input/output and resize prevent a
 // malicious peer from injecting extra fields or mixed instructions.
 schemas:=[][]string{
  append(append([]string(nil),base...),"data"),
  append(append([]string(nil),base...),"rows","cols"),
 }
 var raw map[string]json.RawMessage
 for _,keys:=range schemas{
  raw,err=decodeActionObjectBounded(data,4096,keys...)
  if err==nil{break}
 }
 if err!=nil{ch.Close();return RemotePTYFrame{},denied}
 var w remotePTYFrameWire
 if json.Unmarshal(raw["type"],&w.Type)!=nil||
  json.Unmarshal(raw["request_id"],&w.RequestID)!=nil||
  json.Unmarshal(raw["handle"],&w.Handle)!=nil||
  json.Unmarshal(raw["kind"],&w.Kind)!=nil||
  json.Unmarshal(raw["sequence"],&w.Sequence)!=nil||
  json.Unmarshal(raw["remote_control_available"],&w.RemoteControlAvailable)!=nil||
  w.Type!=remotePTYFrameType||w.RequestID!=id||w.RemoteControlAvailable{
  ch.Close();return RemotePTYFrame{},denied
 }
 if raw["data"]!=nil&&json.Unmarshal(raw["data"],&w.Data)!=nil{
  ch.Close();return RemotePTYFrame{},denied
 }
 if raw["rows"]!=nil&&json.Unmarshal(raw["rows"],&w.Rows)!=nil{
  ch.Close();return RemotePTYFrame{},denied
 }
 if raw["cols"]!=nil&&json.Unmarshal(raw["cols"],&w.Cols)!=nil{
  ch.Close();return RemotePTYFrame{},denied
 }
 frame:=RemotePTYFrame{
  Handle:w.Handle,Kind:w.Kind,Sequence:w.Sequence,
  Data:w.Data,Rows:w.Rows,Cols:w.Cols,
 }
 if !ValidRemotePTYFrame(frame){
  ch.Close()
  for i:=range frame.Data{frame.Data[i]=0}
  return RemotePTYFrame{},denied
 }
 // Caller must separately validate increasing Sequence in the specific
 // owned terminal session. E2EE protects record replay, but per-session
 // sequence guards detach/reconnect ordering.
 return frame,nil
}
