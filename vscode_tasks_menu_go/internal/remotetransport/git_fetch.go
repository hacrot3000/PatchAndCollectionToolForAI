package remotetransport

import (
 "strings"
 "unicode/utf8"
)

// Fixed host-pinned HTTPS Fetch is a separate operator-approved network
// operation. The browser/Relay cannot specify a URL, refspec or credential.
const RemoteActionGitFetchReview="git.fetch_https_review"
const RemoteActionGitFetchApply="git.fetch_https_apply"
const remoteGitFetchReviewResultType="taskdeck-e2ee-git-fetch-https-review-result-v1"
const remoteGitFetchApplyResultType="taskdeck-e2ee-git-fetch-https-apply-result-v1"

func ValidRemoteGitFetchReviewReport(value string)bool{
 if len(value)>1024||!utf8.ValidString(value){return false}
 p:=strings.Split(value,"\n")
 if len(p)!=11||p[0]!="GIT_FETCH_HTTPS_REVIEW_V1"||p[10]!=""||
  p[8]!="WORKTREE clean"||p[9]!="NETWORK HTTPS-approval-required"{return false}
 keys:=[]string{"BRANCH ","HEAD ","UPSTREAM ","UPSTREAM_HEAD ",
  "HTTPS_HOST ","URL_SHA256 ","CONFIG_SHA256 "}
 for i,key:=range keys{
  if !strings.HasPrefix(p[i+1],key){return false}
 }
 branch:=strings.TrimPrefix(p[1],"BRANCH ")
 head:=strings.TrimPrefix(p[2],"HEAD ")
 tracking:=strings.TrimPrefix(p[3],"UPSTREAM ")
 target:=strings.TrimPrefix(p[4],"UPSTREAM_HEAD ")
 host:=strings.TrimPrefix(p[5],"HTTPS_HOST ")
 if !validRemotePullRef(branch,"refs/heads/")||
  !validRemotePullRef(tracking,"refs/remotes/")||
  !validRemoteCommitObjectID(head)||!validRemoteCommitObjectID(target)||
  len(head)!=len(target)||host==""||len(host)>80{return false}
 if host!="github.com"&&host!="gitlab.com"&&host!="bitbucket.org"{return false}
 if !strings.HasPrefix(tracking,"refs/remotes/origin/")||
  strings.TrimPrefix(tracking,"refs/remotes/origin/")!=
   strings.TrimPrefix(branch,"refs/heads/"){return false}
 for _,line:=range p[6:8]{
  value:=line[strings.IndexByte(line,' ')+1:]
  if len(value)!=64{return false}
  for _,c:=range value{
   if !((c>='0'&&c<='9')||(c>='a'&&c<='f')){return false}
  }
 }
 return true
}
func SealRemoteGitFetchReviewResult(ch *Channel,id string,content []byte)([]byte,error){
 if !ValidRemoteGitFetchReviewReport(string(content)){return nil,ErrInvalidRemoteAction}
 return sealRemoteBoundedText4096(ch,id,content,remoteGitFetchReviewResultType)
}
func OpenRemoteGitFetchReviewResult(ch *Channel,frame []byte,id string)(string,error){
 value,err:=openRemoteBoundedText4096(ch,frame,id,remoteGitFetchReviewResultType)
 if err!=nil{return "",err}
 if !ValidRemoteGitFetchReviewReport(value){ch.Close();return "",ErrInvalidRemoteAction}
 return value,nil
}
func SealRemoteGitFetchApplyResult(ch *Channel,id string,content []byte)([]byte,error){
 if !validRemoteCommitObjectID(string(content)){return nil,ErrInvalidRemoteAction}
 return sealRemoteBoundedText4096(ch,id,content,remoteGitFetchApplyResultType)
}
func OpenRemoteGitFetchApplyResult(ch *Channel,frame []byte,id string)(string,error){
 value,err:=openRemoteBoundedText4096(ch,frame,id,remoteGitFetchApplyResultType)
 if err!=nil{return "",err}
 if !validRemoteCommitObjectID(value){ch.Close();return "",ErrInvalidRemoteAction}
 return value,nil
}
