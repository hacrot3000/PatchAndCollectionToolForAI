package remotetransport

import "encoding/json"

const RemoteActionGitDiff="git.diff"
const RemoteGitDiffResultProtocol="taskdeck-e2ee-git-diff-result-v1"

// This operation exposes ONLY a short --stat summary; no arbitrary patches,
// file contents, revisions, paths or external diff helper can be requested.
type remoteGitDiffWire struct{
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 Summary string `json:"summary"`
 RemoteControlAvailable bool `json:"remote_control_available"`
}

func SealRemoteGitDiffResult(ch *Channel,id,summary string)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||
  !ValidRemoteGitLogText(summary){return nil,ErrInvalidRemoteAction}
 raw,err:=json.Marshal(remoteGitDiffWire{
  Type:RemoteGitDiffResultProtocol,RequestID:id,
  Summary:summary,RemoteControlAvailable:false,
 })
 if err!=nil||len(raw)>maxRemoteActionPlaintext{return nil,ErrInvalidRemoteAction}
 return ch.Seal(raw)
}
func OpenRemoteGitDiffResult(ch *Channel,frame []byte,id string)(string,error){
 if ch==nil||!ch.Established()||!validActionID(id){return "",ErrInvalidRemoteAction}
 raw,err:=ch.Open(frame)
 if err!=nil{return "",ErrInvalidRemoteAction}
 defer wipeLoginBytes(raw)
 fields,err:=decodeActionObject(raw,"type","request_id","summary","remote_control_available")
 if err!=nil||string(fields["remote_control_available"])!="false"{
  ch.Close();return "",ErrInvalidRemoteAction
 }
 var typ,requestID,summary string
 if json.Unmarshal(fields["type"],&typ)!=nil||
  json.Unmarshal(fields["request_id"],&requestID)!=nil||
  json.Unmarshal(fields["summary"],&summary)!=nil||
  typ!=RemoteGitDiffResultProtocol||requestID!=id||
  !ValidRemoteGitLogText(summary){
  ch.Close();return "",ErrInvalidRemoteAction
 }
 return summary,nil
}
