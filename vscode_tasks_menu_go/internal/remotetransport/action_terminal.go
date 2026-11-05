package remotetransport

import (
 "bytes"
 "encoding/json"
 "unicode"
 "unicode/utf8"
)

const RemoteActionTerminalOpen="terminal.open"
const RemoteActionTerminalPoll="terminal.poll"
const RemoteActionTerminalWrite="terminal.write"
const RemoteActionTerminalResize="terminal.resize"
const RemoteActionTerminalClose="terminal.close"
const RemoteTerminalProgressProtocol="taskdeck-e2ee-terminal-progress-v1"
const MaxRemoteTerminalInputBytes=96
const MaxRemoteTerminalOutputBytes=128

// The native GUI may transmit a short UTF-8 input segment, including Enter,
// Ctrl-C, Ctrl-D and Backspace. Unknown control/escape sequences are rejected;
// shell parsing is entirely host-side behind terminal-specific TaskDeck RBAC.
func ValidRemoteTerminalInput(s string)bool{
 if len(s)<1||len(s)>MaxRemoteTerminalInputBytes||!utf8.ValidString(s){return false}
 for _,r:=range s{
  switch r{case '\r','\n','\t','\b','\x03','\x04','\x7f':continue}
  if unicode.IsControl(r)||unicode.In(r,unicode.Cf,unicode.Zl,unicode.Zp){
   return false
  }
 }
 return true
}

func ValidRemoteTerminalSize(cols,rows int)bool{
 return cols>=20&&cols<=180&&rows>=8&&rows<=60
}

func terminalOperation(op string)bool{
 return op==RemoteActionTerminalOpen||op==RemoteActionTerminalPoll||
  op==RemoteActionTerminalWrite||op==RemoteActionTerminalResize||
  op==RemoteActionTerminalClose
}

// This response has no transferable terminal ID: precisely one terminal may
// exist per authenticated E2EE session, which is destroyed on revocation.
type RemoteTerminalProgress struct {
 State string
 Output string
 Truncated bool
 More bool
}
type remoteTerminalProgressWire struct{
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 State string `json:"state"`
 Output string `json:"output"`
 Truncated bool `json:"truncated"`
 More bool `json:"more"`
 RemoteControlAvailable bool `json:"remote_control_available"`
}
func validRemoteTerminalProgress(p RemoteTerminalProgress)bool{
 if p.State!="running"&&p.State!="exited"&&p.State!="closed"{return false}
 if len(p.Output)>MaxRemoteTerminalOutputBytes||!utf8.ValidString(p.Output){return false}
 if p.State=="closed"&&(p.More||p.Output!=""){return false}
 for _,r:=range p.Output{
  if r=='
'||r=='	'{continue}
  if unicode.IsControl(r)||unicode.In(r,unicode.Cf,unicode.Zl,unicode.Zp){return false}
 }
 return true
}
func SealRemoteTerminalProgress(ch *Channel,id string,p RemoteTerminalProgress)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||!validRemoteTerminalProgress(p){
  return nil,ErrInvalidRemoteAction
 }
 var buf bytes.Buffer
 enc:=json.NewEncoder(&buf)
 enc.SetEscapeHTML(false)
 if err:=enc.Encode(remoteTerminalProgressWire{
  Type:RemoteTerminalProgressProtocol,RequestID:id,
  State:p.State,Output:p.Output,Truncated:p.Truncated,More:p.More,
  RemoteControlAvailable:false,
 });err!=nil{return nil,ErrInvalidRemoteAction}
 raw:=bytes.TrimSuffix(buf.Bytes(),[]byte{10})
 if len(raw)>maxRemoteActionPlaintext{return nil,ErrInvalidRemoteAction}
 return ch.Seal(raw)
}
func OpenRemoteTerminalProgress(ch *Channel,frame []byte,id string)(RemoteTerminalProgress,error){
 var denied RemoteTerminalProgress
 if ch==nil||!ch.Established()||!validActionID(id){return denied,ErrInvalidRemoteAction}
 clear,err:=ch.Open(frame)
 if err!=nil{return denied,ErrInvalidRemoteAction}
 defer wipeLoginBytes(clear)
 fields,err:=decodeActionObject(clear,"type","request_id","state","output",
  "truncated","more","remote_control_available")
 if err!=nil{
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 var typ,requestID string
 var p RemoteTerminalProgress
 if string(fields["remote_control_available"])!="false"||
  (string(fields["truncated"])!="true"&&string(fields["truncated"])!="false")||
  (string(fields["more"])!="true"&&string(fields["more"])!="false")||
  json.Unmarshal(fields["type"],&typ)!=nil||
  json.Unmarshal(fields["request_id"],&requestID)!=nil||
  json.Unmarshal(fields["state"],&p.State)!=nil||
  json.Unmarshal(fields["output"],&p.Output)!=nil||
  json.Unmarshal(fields["truncated"],&p.Truncated)!=nil||
  json.Unmarshal(fields["more"],&p.More)!=nil||
  typ!=RemoteTerminalProgressProtocol||requestID!=id||
  !validRemoteTerminalProgress(p){
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 return p,nil
}
