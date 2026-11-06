package remotetransport

import (
 "encoding/json"
 "regexp"
 "strings"
)

const RemoteActionGitSwitchReview = "git.switch_review"
const RemoteActionGitSwitchSameHead = "git.switch_same_head"
const RemoteGitSwitchResultProtocol = "taskdeck-e2ee-git-switch-result-v1"

var remoteGitSwitchFromPattern=regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,63}$`)

// The old/current branch may have slash-delimited segments; the host's
// configured target has the stricter single-segment create-branch syntax.
// No client-selected name, ref or checkout arguments are accepted.
func ValidRemoteGitSwitchFrom(value string)bool{
 if !remoteGitSwitchFromPattern.MatchString(value)||value=="HEAD"||
  strings.HasSuffix(value,".")||strings.HasSuffix(value,"/")||
  strings.Contains(value,"..")||strings.Contains(value,"//")||
  strings.Contains(value,"/.")||strings.Contains(value,"@{")||
  strings.HasSuffix(value,".lock"){return false}
 for _,part:=range strings.Split(value,"/"){
  if part=="HEAD"||strings.HasSuffix(part,".lock"){return false}
 }
 return true
}

type RemoteGitSwitchResult struct{
 From string
 Branch string
 Head string
}
type remoteGitSwitchWire struct{
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 From string `json:"from"`
 Branch string `json:"branch"`
 Head string `json:"head"`
 RemoteControlAvailable bool `json:"remote_control_available"`
}
func ValidRemoteGitSwitchResult(value RemoteGitSwitchResult)bool{
 return ValidRemoteGitSwitchFrom(value.From)&&
  ValidRemoteGitCreatedBranchName(value.Branch)&&
  value.From!=value.Branch&&remoteGitPullReviewOID.MatchString(value.Head)
}
func SealRemoteGitSwitchResult(ch *Channel,id string,value RemoteGitSwitchResult)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||
  !ValidRemoteGitSwitchResult(value){return nil,ErrInvalidRemoteAction}
 data,err:=json.Marshal(remoteGitSwitchWire{
  Type:RemoteGitSwitchResultProtocol,RequestID:id,
  From:value.From,Branch:value.Branch,Head:value.Head,
  RemoteControlAvailable:false,
 })
 if err!=nil||len(data)>maxRemoteActionPlaintext{return nil,ErrInvalidRemoteAction}
 return ch.Seal(data)
}
func OpenRemoteGitSwitchResult(ch *Channel,frame []byte,id string)(RemoteGitSwitchResult,error){
 var denied RemoteGitSwitchResult
 if ch==nil||!ch.Established()||!validActionID(id){return denied,ErrInvalidRemoteAction}
 raw,err:=ch.Open(frame)
 if err!=nil{return denied,ErrInvalidRemoteAction}
 defer wipeLoginBytes(raw)
 fields,err:=decodeActionObject(raw,"type","request_id","from","branch","head","remote_control_available")
 if err!=nil||string(fields["remote_control_available"])!="false"{
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 var kind,requestID string
 var value RemoteGitSwitchResult
 if json.Unmarshal(fields["type"],&kind)!=nil||
  json.Unmarshal(fields["request_id"],&requestID)!=nil||
  json.Unmarshal(fields["from"],&value.From)!=nil||
  json.Unmarshal(fields["branch"],&value.Branch)!=nil||
  json.Unmarshal(fields["head"],&value.Head)!=nil||
  kind!=RemoteGitSwitchResultProtocol||requestID!=id||
  !ValidRemoteGitSwitchResult(value){ch.Close();return denied,ErrInvalidRemoteAction}
 return value,nil
}
