package remotetransport

import "encoding/json"

const RemoteActionGitFetch="git.fetch"
const RemoteGitFetchResultProtocol="taskdeck-e2ee-git-fetch-result-v1"

// Fixed Git fetch updates only the host-configured upstream tracking branch.
// No remote URL, credentials, branch name, refspec or shell text is returned.
type RemoteGitFetchResult struct{
 Head string
 Before string
 After string
 Updated bool
}
type remoteGitFetchWire struct{
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 Head string `json:"head"`
 Before string `json:"before"`
 After string `json:"after"`
 Updated bool `json:"updated"`
 RemoteControlAvailable bool `json:"remote_control_available"`
}
func validRemoteGitFetchResult(v RemoteGitFetchResult)bool{
 return gitPullObjectID.MatchString(v.Head)&&
  gitPullObjectID.MatchString(v.Before)&&
  gitPullObjectID.MatchString(v.After)&&v.Updated==(v.Before!=v.After)
}
func SealRemoteGitFetchResult(ch *Channel,id string,v RemoteGitFetchResult)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||
  !validRemoteGitFetchResult(v){return nil,ErrInvalidRemoteAction}
 raw,err:=json.Marshal(remoteGitFetchWire{
  Type:RemoteGitFetchResultProtocol,RequestID:id,
  Head:v.Head,Before:v.Before,After:v.After,
  Updated:v.Updated,RemoteControlAvailable:false,
 })
 if err!=nil||len(raw)>maxRemoteActionPlaintext{return nil,ErrInvalidRemoteAction}
 return ch.Seal(raw)
}
func OpenRemoteGitFetchResult(ch *Channel,frame []byte,id string)(RemoteGitFetchResult,error){
 var denied RemoteGitFetchResult
 if ch==nil||!ch.Established()||!validActionID(id){return denied,ErrInvalidRemoteAction}
 raw,err:=ch.Open(frame)
 if err!=nil{return denied,ErrInvalidRemoteAction}
 defer wipeLoginBytes(raw)
 fields,err:=decodeActionObject(raw,"type","request_id","head","before","after",
  "updated","remote_control_available")
 if err!=nil||string(fields["remote_control_available"])!="false"||
  (string(fields["updated"])!="true"&&string(fields["updated"])!="false"){
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 var typ,requestID string
 var v RemoteGitFetchResult
 if json.Unmarshal(fields["type"],&typ)!=nil||
  json.Unmarshal(fields["request_id"],&requestID)!=nil||
  json.Unmarshal(fields["head"],&v.Head)!=nil||
  json.Unmarshal(fields["before"],&v.Before)!=nil||
  json.Unmarshal(fields["after"],&v.After)!=nil||
  json.Unmarshal(fields["updated"],&v.Updated)!=nil||
  typ!=RemoteGitFetchResultProtocol||requestID!=id||
  !validRemoteGitFetchResult(v){
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 return v,nil
}
