package remotetransport

import (
 "encoding/json"
 "regexp"
)

// Two separate, argument-free E2EE actions. Only an existing TaskDeck-owned
// loose ref at the exact, host configured name is eligible for deletion.
const RemoteActionGitBranchDeleteReview="git.branch_delete.review"
const RemoteActionGitBranchDelete="git.branch_delete"
const RemoteGitBranchDeleteProtocol="taskdeck-e2ee-git-delete-result-v1"

var remoteGitDeleteSlug=regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,39}$`)

func ValidRemoteGitBranchDeleteName(value string)bool{
 const prefix="taskdeck/"
 if len(value)<=len(prefix)||value[:len(prefix)]!=prefix{return false}
 slug:=value[len(prefix):]
 return remoteGitDeleteSlug.MatchString(slug)&&
  slug!="HEAD"&&slug!="main"&&slug!="master"&&slug!="develop"&&
  slug!="trunk"&&slug!="release"&&
  slug[len(slug)-1]!='.'&&
  !regexp.MustCompile(`\\.lockpackage remotetransport

import (
 "encoding/json"
 "regexp"
)

// Two separate, argument-free E2EE actions. Only an existing TaskDeck-owned
// loose ref at the exact, host configured name is eligible for deletion.
const RemoteActionGitBranchDeleteReview="git.branch_delete.review"
const RemoteActionGitBranchDelete="git.branch_delete"
const RemoteGitBranchDeleteProtocol="taskdeck-e2ee-git-delete-result-v1"

var remoteGitDeleteSlug=regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,39}$`)

func ValidRemoteGitBranchDeleteName(value string)bool{
 const prefix="taskdeck/"
 if len(value)<=len(prefix)||value[:len(prefix)]!=prefix{return false}
 slug:=value[len(prefix):]
 return remoteGitDeleteSlug.MatchString(slug)&&
  slug!="HEAD"&&slug!="main"&&slug!="master"&&slug!="develop"&&
  slug!="trunk"&&slug!="release"&&
).MatchString(slug)&&
  len(slug)>0
}

// No path, offset, expected OID, workspace, ref, shell command or force
// flag is ever supplied by a remote client; even the source ref is host-read.
type RemoteGitBranchDeleteResult struct{
 Current string
 Branch string
 Head string
}
type remoteGitBranchDeleteWire struct{
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 Current string `json:"current"`
 Branch string `json:"branch"`
 Head string `json:"head"`
 RemoteControlAvailable bool `json:"remote_control_available"`
}
func ValidRemoteGitBranchDeleteResult(v RemoteGitBranchDeleteResult)bool{
 return ValidRemoteGitSwitchFrom(v.Current)&&
  ValidRemoteGitBranchDeleteName(v.Branch)&&v.Current!=v.Branch&&
  remoteGitPullReviewOID.MatchString(v.Head)
}
func SealRemoteGitBranchDeleteResult(ch *Channel,id string,v RemoteGitBranchDeleteResult)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||
  !ValidRemoteGitBranchDeleteResult(v){return nil,ErrInvalidRemoteAction}
 raw,err:=json.Marshal(remoteGitBranchDeleteWire{
  Type:RemoteGitBranchDeleteProtocol,RequestID:id,
  Current:v.Current,Branch:v.Branch,Head:v.Head,RemoteControlAvailable:false,
 })
 if err!=nil||len(raw)>maxRemoteActionPlaintext{return nil,ErrInvalidRemoteAction}
 return ch.Seal(raw)
}
func OpenRemoteGitBranchDeleteResult(ch *Channel,frame []byte,id string)(RemoteGitBranchDeleteResult,error){
 var denied RemoteGitBranchDeleteResult
 if ch==nil||!ch.Established()||!validActionID(id){return denied,ErrInvalidRemoteAction}
 raw,err:=ch.Open(frame)
 if err!=nil{return denied,ErrInvalidRemoteAction}
 defer wipeLoginBytes(raw)
 fields,err:=decodeActionObject(raw,"type","request_id","current","branch","head","remote_control_available")
 if err!=nil||string(fields["remote_control_available"])!="false"{
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 var typ,reqID string
 var v RemoteGitBranchDeleteResult
 if json.Unmarshal(fields["type"],&typ)!=nil||
  json.Unmarshal(fields["request_id"],&reqID)!=nil||
  json.Unmarshal(fields["current"],&v.Current)!=nil||
  json.Unmarshal(fields["branch"],&v.Branch)!=nil||
  json.Unmarshal(fields["head"],&v.Head)!=nil||
  typ!=RemoteGitBranchDeleteProtocol||reqID!=id||
  !ValidRemoteGitBranchDeleteResult(v){
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 return v,nil
}
