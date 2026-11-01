package remotetransport

import (
 "encoding/json"
 "unicode/utf8"
)

// A bounded, authenticated, read-only byte window of one TaskDeck-owned
// managed process. This is not a PTY, file download or an output stream.
// Cursor values refer to the host's lifetime total, including evicted bytes.
const RemoteManagedOutputWindowMaxBytes=2048
const RemoteManagedOutputMaxOffset=int64(1<<30)
const remoteManagedOutputType="taskdeck-e2ee-managed-output-window-v1"

type RemoteManagedOutput struct {
 Handle string
 Offset int64
 Next int64
 Total int64
 State string
 EOF bool
 Data string
}
type managedOutputWire struct {
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 OK bool `json:"ok"`
 Handle string `json:"handle"`
 Offset int64 `json:"offset"`
 Next int64 `json:"next"`
 Total int64 `json:"total"`
 State string `json:"state"`
 EOF bool `json:"eof"`
 Data string `json:"data"`
 RemoteControlAvailable bool `json:"remote_control_available"`
}

func ValidManagedOutputOffset(n int64)bool{
 return n>=0&&n<=RemoteManagedOutputMaxOffset
}
func ValidRemoteManagedOutput(w RemoteManagedOutput)bool{
 return validManagedOutput(w)
}
func validManagedOutput(w RemoteManagedOutput)bool{
 if !ValidManagedTaskHandle(w.Handle)||
  !ValidManagedOutputOffset(w.Offset)||!ValidManagedOutputOffset(w.Total)||
  w.Offset>w.Next||w.Next>w.Total||len(w.Data)>RemoteManagedOutputWindowMaxBytes||
  w.Next-w.Offset!=int64(len(w.Data))||!utf8.ValidString(w.Data) {
  return false
 }
 if w.State!="running"&&w.State!="exited"&&w.State!="stopped"{return false}
 return w.EOF==(w.State!="running"&&w.Next==w.Total)
}

func SealRemoteManagedOutput(ch *Channel,id string,w RemoteManagedOutput)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||!validManagedOutput(w){
  return nil,ErrInvalidRemoteAction
 }
 raw,err:=json.Marshal(managedOutputWire{
  Type:remoteManagedOutputType,RequestID:id,OK:true,
  Handle:w.Handle,Offset:w.Offset,Next:w.Next,Total:w.Total,
  State:w.State,EOF:w.EOF,Data:w.Data,RemoteControlAvailable:false,
 })
 if err!=nil||len(raw)>RemoteGitDiffMaxPlaintext{return nil,ErrInvalidRemoteAction}
 return ch.Seal(raw)
}
func OpenRemoteManagedOutput(ch *Channel,frame []byte,id string)(RemoteManagedOutput,error){
 denied:=ErrInvalidRemoteAction
 if ch==nil||!ch.Established()||!validActionID(id){return RemoteManagedOutput{},denied}
 rawData,err:=ch.Open(frame)
 if err!=nil{return RemoteManagedOutput{},denied}
 defer wipeLoginBytes(rawData)
 raw,err:=decodeActionObjectBounded(rawData,RemoteGitDiffMaxPlaintext,
  "type","request_id","ok","handle","offset","next","total","state",
  "eof","data","remote_control_available")
 if err!=nil{ch.Close();return RemoteManagedOutput{},denied}
 var v managedOutputWire
 if json.Unmarshal(raw["type"],&v.Type)!=nil||
  json.Unmarshal(raw["request_id"],&v.RequestID)!=nil||
  json.Unmarshal(raw["ok"],&v.OK)!=nil||
  json.Unmarshal(raw["handle"],&v.Handle)!=nil||
  json.Unmarshal(raw["offset"],&v.Offset)!=nil||
  json.Unmarshal(raw["next"],&v.Next)!=nil||
  json.Unmarshal(raw["total"],&v.Total)!=nil||
  json.Unmarshal(raw["state"],&v.State)!=nil||
  json.Unmarshal(raw["eof"],&v.EOF)!=nil||
  json.Unmarshal(raw["data"],&v.Data)!=nil||
  json.Unmarshal(raw["remote_control_available"],&v.RemoteControlAvailable)!=nil||
  v.Type!=remoteManagedOutputType||v.RequestID!=id||!v.OK||
  v.RemoteControlAvailable||!validManagedOutput(RemoteManagedOutput{
   Handle:v.Handle,Offset:v.Offset,Next:v.Next,Total:v.Total,
   State:v.State,EOF:v.EOF,Data:v.Data,
  }){
  ch.Close();return RemoteManagedOutput{},denied
 }
 return RemoteManagedOutput{
  Handle:v.Handle,Offset:v.Offset,Next:v.Next,Total:v.Total,
  State:v.State,EOF:v.EOF,Data:v.Data,
 },nil
}
