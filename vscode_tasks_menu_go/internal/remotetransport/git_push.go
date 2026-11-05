package remotetransport

import "strings"

// A Push action never accepts remote URL, branch, Git argv or credentials
// from PHP, the untrusted browser, or its authenticated native client.
// Only a one-use digest of this canonical, host-generated review crosses
// the E2EE Apply boundary.
const RemoteActionGitPushReview="git.push_https_review"
const RemoteActionGitPushApply="git.push_https_apply"
const remoteGitPushReviewResultType="taskdeck-e2ee-git-push-https-review-result-v1"
const remoteGitPushApplyResultType="taskdeck-e2ee-git-push-https-apply-result-v1"

func ValidRemoteGitPushReviewReport(value string)bool{
 if len(value)>1400{return false}
 p:=strings.Split(value,"\n")
 if len(p)!=15||p[0]!="GIT_PUSH_HTTPS_REVIEW_V1"||p[14]!=""||
  p[10]!="STATUS fast-forward-only"||p[11]!="WORKTREE clean"||
  p[12]!="REMOTE_REF existing"||p[13]!="NETWORK HTTPS-approval-required"{
  return false
 }
 labels:=[]string{
  "BRANCH ","HEAD ","UPSTREAM ","UPSTREAM_HEAD ","HTTPS_HOST ",
  "URL_SHA256 ","CONFIG_SHA256 ","INDEX_SHA256 ","HELPER_SHA256 ",
 }
 for i,k:=range labels{
  if !strings.HasPrefix(p[i+1],k){return false}
 }
 branch:=strings.TrimPrefix(p[1],"BRANCH ")
 tracking:=strings.TrimPrefix(p[3],"UPSTREAM ")
 head:=strings.TrimPrefix(p[2],"HEAD ")
 old:=strings.TrimPrefix(p[4],"UPSTREAM_HEAD ")
 host:=strings.TrimPrefix(p[5],"HTTPS_HOST ")
 if !validRemotePullRef(branch,"refs/heads/")||
  !validRemotePullRef(tracking,"refs/remotes/")||
  tracking!="refs/remotes/origin/"+strings.TrimPrefix(branch,"refs/heads/")||
  !validRemoteCommitObjectID(head)||!validRemoteCommitObjectID(old)||
  len(head)!=len(old)||head==old||
  (host!="github.com"&&host!="gitlab.com"&&host!="bitbucket.org"){
  return false
 }
 for _,line:=range p[6:10]{
  digest:=line[strings.IndexByte(line,' ')+1:]
  if len(digest)!=64{return false}
  for _,c:=range digest{
   if !((c>='0'&&c<='9')||(c>='a'&&c<='f')){return false}
  }
 }
 return true
}
func SealRemoteGitPushReviewResult(ch *Channel,id string,content []byte)([]byte,error){
 if !ValidRemoteGitPushReviewReport(string(content)){return nil,ErrInvalidRemoteAction}
 return sealRemoteBoundedText4096(ch,id,content,remoteGitPushReviewResultType)
}
func OpenRemoteGitPushReviewResult(ch *Channel,frame []byte,id string)(string,error){
 value,err:=openRemoteBoundedText4096(ch,frame,id,remoteGitPushReviewResultType)
 if err!=nil{return "",err}
 if !ValidRemoteGitPushReviewReport(value){ch.Close();return "",ErrInvalidRemoteAction}
 return value,nil
}
func SealRemoteGitPushApplyResult(ch *Channel,id string,content []byte)([]byte,error){
 if !validRemoteCommitObjectID(string(content)){return nil,ErrInvalidRemoteAction}
 return sealRemoteBoundedText4096(ch,id,content,remoteGitPushApplyResultType)
}
func OpenRemoteGitPushApplyResult(ch *Channel,frame []byte,id string)(string,error){
 value,err:=openRemoteBoundedText4096(ch,frame,id,remoteGitPushApplyResultType)
 if err!=nil{return "",err}
 if !validRemoteCommitObjectID(value){ch.Close();return "",ErrInvalidRemoteAction}
 return value,nil
}
