package remotetransport

import (
 "encoding/json"
 "regexp"
)

const RemoteActionGitPull = "git.pull"
const RemoteGitPullResultProtocol = "taskdeck-e2ee-git-pull-result-v1"

// Only the two authenticated object IDs are returned. Neither the remote
// URL, Git command, credentials nor the host's absolute workspace are sent.
type RemoteGitPullResult struct {
 Before string
 After string
 Updated bool
}
type remoteGitPullWire struct {
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 Before string `json:"before"`
 After string `json:"after"`
 Updated bool `json:"updated"`
 RemoteControlAvailable bool `json:"remote_control_available"`
}
var gitPullObjectID = regexp.MustCompile(`^[0-9a-f]{40}$`)

func validRemoteGitPullResult(value RemoteGitPullResult) bool {
 return gitPullObjectID.MatchString(value.Before) &&
  gitPullObjectID.MatchString(value.After) &&
  value.Updated==(value.Before!=value.After)
}

func SealRemoteGitPullResult(ch *Channel,id string,value RemoteGitPullResult)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||!validRemoteGitPullResult(value){
  return nil,ErrInvalidRemoteAction
 }
 raw,err:=json.Marshal(remoteGitPullWire{
  Type:RemoteGitPullResultProtocol,RequestID:id,
  Before:value.Before,After:value.After,Updated:value.Updated,
  RemoteControlAvailable:false,
 })
 if err!=nil||len(raw)>maxRemoteActionPlaintext{return nil,ErrInvalidRemoteAction}
 return ch.Seal(raw)
}

func OpenRemoteGitPullResult(ch *Channel,frame []byte,id string)(RemoteGitPullResult,error){
 var denied RemoteGitPullResult
 if ch==nil||!ch.Established()||!validActionID(id){return denied,ErrInvalidRemoteAction}
 raw,err:=ch.Open(frame)
 if err!=nil{return denied,ErrInvalidRemoteAction}
 defer wipeLoginBytes(raw)
 fields,err:=decodeActionObject(raw,"type","request_id","before","after",
  "updated","remote_control_available")
 if err!=nil{
  ch.Close()
  return denied,ErrInvalidRemoteAction
 }
 if string(fields["remote_control_available"])!="false"||
  (string(fields["updated"])!="true"&&string(fields["updated"])!="false"){
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 var kind,requestID string
 var result RemoteGitPullResult
 if json.Unmarshal(fields["type"],&kind)!=nil||
  json.Unmarshal(fields["request_id"],&requestID)!=nil||
  json.Unmarshal(fields["before"],&result.Before)!=nil||
  json.Unmarshal(fields["after"],&result.After)!=nil||
  json.Unmarshal(fields["updated"],&result.Updated)!=nil||
  kind!=RemoteGitPullResultProtocol||requestID!=id||
  !validRemoteGitPullResult(result){
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 return result,nil
}
