package remotetransport

import (
 "encoding/json"
 "regexp"
)

const RemoteActionGitPullReview = "git.pull_review"
const RemoteGitPullReviewResultProtocol = "taskdeck-e2ee-git-pull-review-v1"

// This is a read-only preview of already-fetched, local Git objects.
// It never grants permission to fetch, pull, modify refs, or update files.
type RemoteGitPullReviewResult struct {
 Head string
 UpstreamHead string
 Status string
}
type remoteGitPullReviewWire struct {
 Type string `json:"type"`
 RequestID string `json:"request_id"`
 Head string `json:"head"`
 UpstreamHead string `json:"upstream_head"`
 Status string `json:"status"`
 RemoteControlAvailable bool `json:"remote_control_available"`
}
var remoteGitPullReviewOID=regexp.MustCompile("^[0-9a-f]{40}$")

func validRemoteGitPullReviewResult(v RemoteGitPullReviewResult)bool {
 return remoteGitPullReviewOID.MatchString(v.Head) &&
  remoteGitPullReviewOID.MatchString(v.UpstreamHead) &&
  (v.Status=="up-to-date"&&v.Head==v.UpstreamHead||
   v.Status=="fast-forward"&&v.Head!=v.UpstreamHead)
}
func SealRemoteGitPullReviewResult(ch *Channel,id string,v RemoteGitPullReviewResult)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||!validRemoteGitPullReviewResult(v){
  return nil,ErrInvalidRemoteAction
 }
 plain,err:=json.Marshal(remoteGitPullReviewWire{
  Type:RemoteGitPullReviewResultProtocol,RequestID:id,
  Head:v.Head,UpstreamHead:v.UpstreamHead,Status:v.Status,
  RemoteControlAvailable:false,
 })
 if err!=nil||len(plain)>maxRemoteActionPlaintext{return nil,ErrInvalidRemoteAction}
 return ch.Seal(plain)
}
func OpenRemoteGitPullReviewResult(ch *Channel,frame []byte,id string)(RemoteGitPullReviewResult,error){
 var denied RemoteGitPullReviewResult
 if ch==nil||!ch.Established()||!validActionID(id){return denied,ErrInvalidRemoteAction}
 plain,err:=ch.Open(frame)
 if err!=nil{return denied,ErrInvalidRemoteAction}
 defer wipeLoginBytes(plain)
 fields,err:=decodeActionObject(plain,"type","request_id","head","upstream_head",
  "status","remote_control_available")
 if err!=nil||string(fields["remote_control_available"])!="false"{
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 var kind,got string
 var result RemoteGitPullReviewResult
 if json.Unmarshal(fields["type"],&kind)!=nil||
  json.Unmarshal(fields["request_id"],&got)!=nil||
  json.Unmarshal(fields["head"],&result.Head)!=nil||
  json.Unmarshal(fields["upstream_head"],&result.UpstreamHead)!=nil||
  json.Unmarshal(fields["status"],&result.Status)!=nil||
  kind!=RemoteGitPullReviewResultProtocol||got!=id||
  !validRemoteGitPullReviewResult(result){
  ch.Close();return denied,ErrInvalidRemoteAction
 }
 return result,nil
}
