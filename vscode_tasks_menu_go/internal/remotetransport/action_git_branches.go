package remotetransport

import "encoding/json"

// Git branches is a fixed, read-only list of no more than four local branch
// labels. It never selects, creates, checks out, fetches or deletes a ref.
const RemoteActionGitBranches="git.branches"
const RemoteGitBranchesResultProtocol="taskdeck-e2ee-git-branches-result-v1"

type remoteGitBranchesWire struct{
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 Branches string `json:"branches"`
 RemoteControlAvailable bool `json:"remote_control_available"`
}

func SealRemoteGitBranchesResult(ch *Channel,id,branches string)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||
  !ValidRemoteGitLogText(branches){return nil,ErrInvalidRemoteAction}
 raw,err:=json.Marshal(remoteGitBranchesWire{
  Type:RemoteGitBranchesResultProtocol,RequestID:id,
  Branches:branches,RemoteControlAvailable:false,
 })
 if err!=nil||len(raw)>maxRemoteActionPlaintext{return nil,ErrInvalidRemoteAction}
 return ch.Seal(raw)
}

func OpenRemoteGitBranchesResult(ch *Channel,frame []byte,id string)(string,error){
 if ch==nil||!ch.Established()||!validActionID(id){return "",ErrInvalidRemoteAction}
 raw,err:=ch.Open(frame)
 if err!=nil{return "",ErrInvalidRemoteAction}
 defer wipeLoginBytes(raw)
 fields,err:=decodeActionObject(raw,"type","request_id","branches",
  "remote_control_available")
 if err!=nil||string(fields["remote_control_available"])!="false"{
  ch.Close();return "",ErrInvalidRemoteAction
 }
 var typ,requestID,branches string
 if json.Unmarshal(fields["type"],&typ)!=nil||
  json.Unmarshal(fields["request_id"],&requestID)!=nil||
  json.Unmarshal(fields["branches"],&branches)!=nil||
  typ!=RemoteGitBranchesResultProtocol||requestID!=id||
  !ValidRemoteGitLogText(branches){
  ch.Close();return "",ErrInvalidRemoteAction
 }
 return branches,nil
}
