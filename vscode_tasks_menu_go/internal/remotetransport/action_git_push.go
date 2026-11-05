package remotetransport

import "encoding/json"

const RemoteActionGitPush = "git.push"
const RemoteGitPushResultProtocol = "taskdeck-e2ee-git-push-result-v1"

// A successful push only reports the exact local commit that the Agent
// submitted to the host-configured upstream. Branch names, remote URLs,
// credentials, command output and Git internals never cross E2EE.
type RemoteGitPushResult struct{ Head string }

type remoteGitPushWire struct {
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 Head string `json:"head"`
 RemoteControlAvailable bool `json:"remote_control_available"`
}

func SealRemoteGitPushResult(ch *Channel,id string,value RemoteGitPushResult)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||
  !gitPullObjectID.MatchString(value.Head){return nil,ErrInvalidRemoteAction}
 raw,err:=json.Marshal(remoteGitPushWire{
  Type:RemoteGitPushResultProtocol,RequestID:id,
  Head:value.Head,RemoteControlAvailable:false,
 })
 if err!=nil||len(raw)>maxRemoteActionPlaintext{return nil,ErrInvalidRemoteAction}
 return ch.Seal(raw)
}

func OpenRemoteGitPushResult(ch *Channel,frame []byte,id string)(RemoteGitPushResult,error){
 var denied RemoteGitPushResult
 if ch==nil||!ch.Established()||!validActionID(id){return denied,ErrInvalidRemoteAction}
 raw,err:=ch.Open(frame)
 if err!=nil{return denied,ErrInvalidRemoteAction}
 defer wipeLoginBytes(raw)
 fields,err:=decodeActionObject(raw,"type","request_id","head",
  "remote_control_available")
 if err!=nil||string(fields["remote_control_available"])!="false"{
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 var typ,requestID string
 var result RemoteGitPushResult
 if json.Unmarshal(fields["type"],&typ)!=nil||
  json.Unmarshal(fields["request_id"],&requestID)!=nil||
  json.Unmarshal(fields["head"],&result.Head)!=nil||
  typ!=RemoteGitPushResultProtocol||requestID!=id||
  !gitPullObjectID.MatchString(result.Head){
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 return result,nil
}
