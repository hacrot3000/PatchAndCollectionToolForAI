package remotetransport

import "encoding/json"

const RemoteActionGitStageTracked="git.stage_tracked"
const RemoteActionGitUnstageAll="git.unstage_all"
const RemoteGitIndexResultProtocol="taskdeck-e2ee-git-index-result-v1"

// Exactly two host-owned index operations exist: one stages changes in
// already-tracked files, the other resets only the staging index to HEAD.
// Neither command accepts remote-provided paths, file contents or argv.
type RemoteGitIndexResult struct{
 Operation string
 Head string
}
type remoteGitIndexResultWire struct{
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 Operation string `json:"operation"`
 Head string `json:"head"`
 RemoteControlAvailable bool `json:"remote_control_available"`
}
func validRemoteGitIndexOp(op string)bool{
 return op==RemoteActionGitStageTracked||op==RemoteActionGitUnstageAll
}
func SealRemoteGitIndexResult(ch *Channel,id string,v RemoteGitIndexResult)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||
  !validRemoteGitIndexOp(v.Operation)||!gitPullObjectID.MatchString(v.Head){
  return nil,ErrInvalidRemoteAction
 }
 raw,err:=json.Marshal(remoteGitIndexResultWire{
  Type:RemoteGitIndexResultProtocol,RequestID:id,
  Operation:v.Operation,Head:v.Head,RemoteControlAvailable:false,
 })
 if err!=nil||len(raw)>maxRemoteActionPlaintext{return nil,ErrInvalidRemoteAction}
 return ch.Seal(raw)
}
func OpenRemoteGitIndexResult(ch *Channel,frame []byte,id,wantOp string)(RemoteGitIndexResult,error){
 var denied RemoteGitIndexResult
 if ch==nil||!ch.Established()||!validActionID(id)||!validRemoteGitIndexOp(wantOp){
  return denied,ErrInvalidRemoteAction
 }
 raw,err:=ch.Open(frame)
 if err!=nil{return denied,ErrInvalidRemoteAction}
 defer wipeLoginBytes(raw)
 fields,err:=decodeActionObject(raw,"type","request_id","operation","head",
  "remote_control_available")
 if err!=nil||string(fields["remote_control_available"])!="false"{
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 var typ,requestID string
 var result RemoteGitIndexResult
 if json.Unmarshal(fields["type"],&typ)!=nil||
  json.Unmarshal(fields["request_id"],&requestID)!=nil||
  json.Unmarshal(fields["operation"],&result.Operation)!=nil||
  json.Unmarshal(fields["head"],&result.Head)!=nil||
  typ!=RemoteGitIndexResultProtocol||requestID!=id||
  result.Operation!=wantOp||!gitPullObjectID.MatchString(result.Head){
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 return result,nil
}
