package remotetransport

import (
 "encoding/json"
 "regexp"
 "strings"
)

const RemoteActionGitBranchCreate="git.branch_create"
const RemoteGitBranchCreateResultProtocol="taskdeck-e2ee-git-branch-create-result-v1"
var remoteNewGitBranchName=regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// These are display-only labels selected by an administrator at the HOST.
// The native GUI and untrusted Portal cannot specify a name, ref or argv.
func ValidRemoteGitCreatedBranchName(name string)bool{
 return remoteNewGitBranchName.MatchString(name)&&
  name!="HEAD"&&name!="@"&&name!="-"&&
  !strings.HasSuffix(name,".")&&!strings.HasSuffix(name,".lock")&&
  !strings.Contains(name,"..")&&!strings.Contains(name,"@{")
}

type RemoteGitBranchCreateResult struct{
 Branch string
 Head string
}
type remoteGitBranchCreateWire struct{
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 Branch string `json:"branch"`
 Head string `json:"head"`
 RemoteControlAvailable bool `json:"remote_control_available"`
}
func SealRemoteGitBranchCreateResult(ch *Channel,id string,v RemoteGitBranchCreateResult)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||
  !ValidRemoteGitCreatedBranchName(v.Branch)||!gitPullObjectID.MatchString(v.Head){
  return nil,ErrInvalidRemoteAction
 }
 raw,err:=json.Marshal(remoteGitBranchCreateWire{
  Type:RemoteGitBranchCreateResultProtocol,
  RequestID:id,Branch:v.Branch,Head:v.Head,RemoteControlAvailable:false,
 })
 if err!=nil||len(raw)>maxRemoteActionPlaintext{return nil,ErrInvalidRemoteAction}
 return ch.Seal(raw)
}
func OpenRemoteGitBranchCreateResult(ch *Channel,frame []byte,id string)(RemoteGitBranchCreateResult,error){
 var denied RemoteGitBranchCreateResult
 if ch==nil||!ch.Established()||!validActionID(id){return denied,ErrInvalidRemoteAction}
 raw,err:=ch.Open(frame)
 if err!=nil{return denied,ErrInvalidRemoteAction}
 defer wipeLoginBytes(raw)
 fields,err:=decodeActionObject(raw,"type","request_id","branch","head",
  "remote_control_available")
 if err!=nil||string(fields["remote_control_available"])!="false"{
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 var typ,requestID string
 var v RemoteGitBranchCreateResult
 if json.Unmarshal(fields["type"],&typ)!=nil||
  json.Unmarshal(fields["request_id"],&requestID)!=nil||
  json.Unmarshal(fields["branch"],&v.Branch)!=nil||
  json.Unmarshal(fields["head"],&v.Head)!=nil||
  typ!=RemoteGitBranchCreateResultProtocol||requestID!=id||
  !ValidRemoteGitCreatedBranchName(v.Branch)||!gitPullObjectID.MatchString(v.Head){
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 return v,nil
}
