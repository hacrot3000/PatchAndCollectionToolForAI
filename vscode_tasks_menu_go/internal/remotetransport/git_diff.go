package remotetransport

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "strings"
 "unicode/utf8"
)

// Separate authenticated, 4KiB read-only result types. These NEVER grant
// write, shell or file-transfer authority. A compromised Relay sees only AEAD.
const RemoteGitDiffMaxBytes=4096
const RemoteGitDiffMaxPlaintext=6144
const MaxEncryptedGitDiffFrame=RemoteGitDiffMaxPlaintext+9+16
const remoteGitDiffResultType="taskdeck-e2ee-git-diff-result-v1"
const remoteGitLogResultType="taskdeck-e2ee-git-log-result-v1"
const remoteGitBranchesResultType="taskdeck-e2ee-git-branches-result-v1"
const remoteGitRemotesResultType="taskdeck-e2ee-git-remotes-result-v1"
const remoteGitTrackingRefsResultType="taskdeck-e2ee-git-tracking-refs-result-v1"
const remoteGitPullReviewResultType="taskdeck-e2ee-git-pull-review-result-v1"
const remoteGitPullCachedReviewResultType="taskdeck-e2ee-git-pull-cached-review-result-v1"
const remoteGitPullApplyCachedResultType="taskdeck-e2ee-git-pull-cached-apply-result-v1"
const remoteGitPullApplyFilesResultType="taskdeck-e2ee-git-pull-files-apply-result-v1"
const remoteGitStagedDiffResultType="taskdeck-e2ee-git-diff-staged-result-v1"
const remoteGitHeadDiffResultType="taskdeck-e2ee-git-diff-head-result-v1"
const remoteGitStageResultType="taskdeck-e2ee-git-stage-result-v1"
const remoteGitUnstageResultType="taskdeck-e2ee-git-unstage-result-v1"
const remoteTaskListResultType="taskdeck-e2ee-task-list-result-v1"
const remoteTaskRunTextResultType="taskdeck-e2ee-task-run-text-result-v1"
const remoteGitStatusFullResultType="taskdeck-e2ee-git-status-full-result-v1"
const remoteGitCommitReviewResultType="taskdeck-e2ee-git-commit-review-result-v1"
const remoteGitCommitResultType="taskdeck-e2ee-git-commit-result-v1"
const remoteGitBranchReviewResultType="taskdeck-e2ee-git-branch-review-result-v1"
const remoteGitBranchCreateResultType="taskdeck-e2ee-git-branch-create-result-v1"
const remoteGitSwitchReviewResultType="taskdeck-e2ee-git-switch-review-result-v1"
const remoteGitSwitchResultType="taskdeck-e2ee-git-switch-result-v1"
const remoteGitBranchDeleteReviewResultType="taskdeck-e2ee-git-branch-delete-review-result-v1"
const remoteGitBranchDeleteResultType="taskdeck-e2ee-git-branch-delete-result-v1"
const RemoteFilePreviewMaxBytes=4096
const remoteFilePreviewResultType="taskdeck-e2ee-file-preview-result-v1"
const remoteFileLinesResultType="taskdeck-e2ee-file-lines-result-v1"
const remoteFindLinesResultType="taskdeck-e2ee-file-find-lines-result-v1"

func SealRemoteGitDiffResult(ch *Channel,id string,content []byte)([]byte,error){
 return sealRemoteBoundedText4096(ch,id,content,remoteGitDiffResultType)
}

func OpenRemoteGitDiffResult(ch *Channel,frame []byte,id string)(string,error){
 return openRemoteBoundedText4096(ch,frame,id,remoteGitDiffResultType)
}

// Preview reads only a COMPLETE <=4KiB visible UTF-8 file. It is not a
// binary window/download and does not transition into file-transfer mode.
func SealRemoteFindLinesResult(ch *Channel,id string,content []byte)([]byte,error){
 return sealRemoteBoundedText4096(ch,id,content,remoteFindLinesResultType)
}
func OpenRemoteFindLinesResult(ch *Channel,frame []byte,id string)(string,error){
 return openRemoteBoundedText4096(ch,frame,id,remoteFindLinesResultType)
}
func SealRemoteFileLinesResult(ch *Channel,id string,content []byte)([]byte,error){
 return sealRemoteBoundedText4096(ch,id,content,remoteFileLinesResultType)
}
func OpenRemoteFileLinesResult(ch *Channel,frame []byte,id string)(string,error){
 return openRemoteBoundedText4096(ch,frame,id,remoteFileLinesResultType)
}
func SealRemoteFilePreviewResult(ch *Channel,id string,content []byte)([]byte,error){
 return sealRemoteBoundedText4096(ch,id,content,remoteFilePreviewResultType)
}
func OpenRemoteFilePreviewResult(ch *Channel,frame []byte,id string)(string,error){
 return openRemoteBoundedText4096(ch,frame,id,remoteFilePreviewResultType)
}

func SealRemoteGitLogResult(ch *Channel,id string,content []byte)([]byte,error){
 return sealRemoteBoundedText4096(ch,id,content,remoteGitLogResultType)
}
func OpenRemoteGitLogResult(ch *Channel,frame []byte,id string)(string,error){
 return openRemoteBoundedText4096(ch,frame,id,remoteGitLogResultType)
}
func SealRemoteGitBranchesResult(ch *Channel,id string,content []byte)([]byte,error){
 return sealRemoteBoundedText4096(ch,id,content,remoteGitBranchesResultType)
}
func OpenRemoteGitBranchesResult(ch *Channel,frame []byte,id string)(string,error){
 return openRemoteBoundedText4096(ch,frame,id,remoteGitBranchesResultType)
}
// Separate AEAD result type prevents replaying unrelated Git metadata.
func SealRemoteGitRemotesResult(ch *Channel,id string,content []byte)([]byte,error){
 return sealRemoteBoundedText4096(ch,id,content,remoteGitRemotesResultType)
}
func OpenRemoteGitRemotesResult(ch *Channel,frame []byte,id string)(string,error){
 value,err:=openRemoteBoundedText4096(ch,frame,id,remoteGitRemotesResultType)
 if err!=nil{return "",err}
 if !ValidRemoteGitRemotesReport(value){ch.Close();return "",ErrInvalidRemoteAction}
 return value,nil
}
// The host and client both fail closed on malformed remote-name lists. Never
// parse credential-bearing URLs or allow arbitrary name/path/option syntax.
func ValidRemoteGitRemotesReport(value string)bool{
 const prefix="GIT_REMOTES_V1\n"
 if !strings.HasPrefix(value,prefix){return false}
 rest:=strings.TrimPrefix(value,prefix)
 if rest==""{return true}
 if len(rest)>1024||!strings.HasSuffix(rest,"\n"){return false}
 names:=strings.Split(strings.TrimSuffix(rest,"\n"),"\n")
 if len(names)>16{return false}
 seen:=map[string]bool{}
 for _,name:=range names{
  if len(name)<1||len(name)>64||seen[name]{return false}
  for i:=0;i<len(name);i++{
   c:=name[i]
   alnum:=(c>='A'&&c<='Z')||(c>='a'&&c<='z')||(c>='0'&&c<='9')
   if alnum{continue}
   if i==0||i==len(name)-1||!(c=='.'||c=='-'||c=='_'){return false}
  }
  if strings.Contains(name,".."){return false}
  seen[name]=true
 }
 return true
}
func SealRemoteGitTrackingRefsResult(ch *Channel,id string,content []byte)([]byte,error){
 if !ValidRemoteGitTrackingRefsReport(string(content)){return nil,ErrInvalidRemoteAction}
 return sealRemoteBoundedText4096(ch,id,content,remoteGitTrackingRefsResultType)
}
func OpenRemoteGitTrackingRefsResult(ch *Channel,frame []byte,id string)(string,error){
 value,err:=openRemoteBoundedText4096(ch,frame,id,remoteGitTrackingRefsResultType)
 if err!=nil{return "",err}
 if !ValidRemoteGitTrackingRefsReport(value){ch.Close();return "",ErrInvalidRemoteAction}
 return value,nil
}
func ValidRemoteGitTrackingRefsReport(value string)bool{
 const prefix="GIT_TRACKING_V1\n"
 if !strings.HasPrefix(value,prefix){return false}
 rest:=strings.TrimPrefix(value,prefix)
 if rest==""{return true}
 if len(rest)>4096||!strings.HasSuffix(rest,"\n"){return false}
 lines:=strings.Split(strings.TrimSuffix(rest,"\n"),"\n")
 if len(lines)>32{return false}
 seen:=map[string]bool{}
 for _,line:=range lines{
  i:=strings.LastIndexByte(line,' ')
  if i<3||i>=len(line)-1||strings.Contains(line[:i]," "){return false}
  name,oid:=line[:i],line[i+1:]
  if len(name)>160||seen[name]||strings.HasSuffix(name,"/HEAD"){return false}
  segments:=strings.Split(name,"/")
  if len(segments)<2||len(segments)>7{return false}
  for _,part:=range segments{
   if len(part)<1||len(part)>64||strings.Contains(part,"..")||
    strings.HasSuffix(part,".lock"){return false}
   for j:=0;j<len(part);j++{
    c:=part[j]
    alnum:=(c>='a'&&c<='z')||(c>='A'&&c<='Z')||(c>='0'&&c<='9')
    if alnum{continue}
    if j==0||j==len(part)-1||!(c=='_'||c=='-'||c=='.'){return false}
   }
  }
  if len(oid)!=40&&len(oid)!=64{return false}
  for j:=0;j<len(oid);j++{
   if !(oid[j]>='0'&&oid[j]<='9')&&!(oid[j]>='a'&&oid[j]<='f'){return false}
  }
  seen[name]=true
 }
 return true
}
// Fixed, OFFLINE Pull preflight; a completely different authenticated
// domain from a future Git write. No network address, credential or refspec.
func SealRemoteGitPullReviewResult(ch *Channel,id string,content []byte)([]byte,error){
 if !ValidRemoteGitPullReviewReport(string(content)){return nil,ErrInvalidRemoteAction}
 return sealRemoteBoundedText4096(ch,id,content,remoteGitPullReviewResultType)
}
func OpenRemoteGitPullReviewResult(ch *Channel,frame []byte,id string)(string,error){
 value,err:=openRemoteBoundedText4096(ch,frame,id,remoteGitPullReviewResultType)
 if err!=nil{return "",err}
 if !ValidRemoteGitPullReviewReport(value){ch.Close();return "",ErrInvalidRemoteAction}
 return value,nil
}
func validRemotePullRef(ref,prefix string)bool{
 if !strings.HasPrefix(ref,prefix)||len(ref)>180{return false}
 parts:=strings.Split(strings.TrimPrefix(ref,prefix),"/")
 if len(parts)<1||len(parts)>7{return false}
 if prefix=="refs/remotes/"&&len(parts)<2{return false}
 for _,p:=range parts{
  if len(p)<1||len(p)>64||strings.HasSuffix(p,".lock")||
   p=="HEAD"||strings.Contains(p,".."){return false}
  for i:=0;i<len(p);i++{
   b:=p[i]
   ok:=(b>='a'&&b<='z')||(b>='A'&&b<='Z')||(b>='0'&&b<='9')
   if ok{continue}
   if i==0||i==len(p)-1||(b!='.'&&b!='-'&&b!='_'){return false}
  }
 }
 return true
}
func ValidRemoteGitPullReviewReport(v string)bool{
 if len(v)>1024||!utf8.ValidString(v){return false}
 p:=strings.Split(v,"\n")
 if len(p)!=9||p[0]!="GIT_PULL_REVIEW_V1"||p[8]!=""||
  p[6]!="WORKTREE clean"||p[7]!="NETWORK none"{return false}
 branch:=strings.TrimPrefix(p[1],"BRANCH ")
 upstream:=strings.TrimPrefix(p[3],"UPSTREAM ")
 head:=strings.TrimPrefix(p[2],"HEAD ")
 target:=strings.TrimPrefix(p[4],"UPSTREAM_HEAD ")
 if !strings.HasPrefix(p[1],"BRANCH ")||
  !strings.HasPrefix(p[2],"HEAD ")||
  !strings.HasPrefix(p[3],"UPSTREAM ")||
  !strings.HasPrefix(p[4],"UPSTREAM_HEAD ")||
  !validRemotePullRef(branch,"refs/heads/")||
  !validRemotePullRef(upstream,"refs/remotes/")||
  !validRemoteCommitObjectID(head)||!validRemoteCommitObjectID(target)||
  len(head)!=len(target){return false}
 switch p[5]{
 case "STATUS up-to-date":return head==target
 case "STATUS fast-forward":return head!=target
 default:return false
 }
}
// Read-only two-step review for ref-only, already-fetched fast-forward.
// Result domain differs from both ordinary Pull preflight and Git write.
func ValidRemoteGitPullCachedReviewReport(v string)bool{
 if len(v)>1024||!utf8.ValidString(v){return false}
 lines:=strings.Split(v,"\n")
 if len(lines)!=10||lines[0]!="GIT_PULL_CACHED_V1"||lines[9]!=""||
  lines[6]!="TREE unchanged"||lines[7]!="WORKTREE clean"||
  lines[8]!="NETWORK none"{return false}
 if !strings.HasPrefix(lines[1],"BRANCH ")||
  !strings.HasPrefix(lines[2],"HEAD ")||
  !strings.HasPrefix(lines[3],"UPSTREAM ")||
  !strings.HasPrefix(lines[4],"UPSTREAM_HEAD ")||
  !strings.HasPrefix(lines[5],"INDEX_SHA256 "){return false}
 branch:=strings.TrimPrefix(lines[1],"BRANCH ")
 head:=strings.TrimPrefix(lines[2],"HEAD ")
 upstream:=strings.TrimPrefix(lines[3],"UPSTREAM ")
 target:=strings.TrimPrefix(lines[4],"UPSTREAM_HEAD ")
 index:=strings.TrimPrefix(lines[5],"INDEX_SHA256 ")
 if !validRemotePullRef(branch,"refs/heads/")||
  !validRemotePullRef(upstream,"refs/remotes/")||
  !validRemoteCommitObjectID(head)||!validRemoteCommitObjectID(target)||
  len(head)!=len(target)||head==target||len(index)!=64{return false}
 for _,b:=range index{if !((b>='0'&&b<='9')||(b>='a'&&b<='f')){return false}}
 return true
}
func SealRemoteGitPullCachedReviewResult(ch *Channel,id string,content []byte)([]byte,error){
 if !ValidRemoteGitPullCachedReviewReport(string(content)){return nil,ErrInvalidRemoteAction}
 return sealRemoteBoundedText4096(ch,id,content,remoteGitPullCachedReviewResultType)
}
func OpenRemoteGitPullCachedReviewResult(ch *Channel,frame []byte,id string)(string,error){
 value,err:=openRemoteBoundedText4096(ch,frame,id,remoteGitPullCachedReviewResultType)
 if err!=nil{return "",err}
 if !ValidRemoteGitPullCachedReviewReport(value){ch.Close();return "",ErrInvalidRemoteAction}
 return value,nil
}
func SealRemoteGitPullApplyCachedResult(ch *Channel,id string,content []byte)([]byte,error){
 if !validRemoteCommitObjectID(string(content)){return nil,ErrInvalidRemoteAction}
 return sealRemoteBoundedText4096(ch,id,content,remoteGitPullApplyCachedResultType)
}
func OpenRemoteGitPullApplyCachedResult(ch *Channel,frame []byte,id string)(string,error){
 value,err:=openRemoteBoundedText4096(ch,frame,id,remoteGitPullApplyCachedResultType)
 if err!=nil{return "",err}
 if !validRemoteCommitObjectID(value){ch.Close();return "",ErrInvalidRemoteAction}
 return value,nil
}
func SealRemoteGitPullApplyFilesResult(ch *Channel,id string,content []byte)([]byte,error){
 if !validRemoteCommitObjectID(string(content)){return nil,ErrInvalidRemoteAction}
 return sealRemoteBoundedText4096(ch,id,content,remoteGitPullApplyFilesResultType)
}
func OpenRemoteGitPullApplyFilesResult(ch *Channel,frame []byte,id string)(string,error){
 value,err:=openRemoteBoundedText4096(ch,frame,id,remoteGitPullApplyFilesResultType)
 if err!=nil{return "",err}
 if !validRemoteCommitObjectID(value){ch.Close();return "",ErrInvalidRemoteAction}
 return value,nil
}
func SealRemoteGitStagedDiffResult(ch *Channel,id string,content []byte)([]byte,error){
 return sealRemoteBoundedText4096(ch,id,content,remoteGitStagedDiffResultType)
}
func OpenRemoteGitStagedDiffResult(ch *Channel,frame []byte,id string)(string,error){
 return openRemoteBoundedText4096(ch,frame,id,remoteGitStagedDiffResultType)
}
func SealRemoteGitHeadDiffResult(ch *Channel,id string,content []byte)([]byte,error){
 return sealRemoteBoundedText4096(ch,id,content,remoteGitHeadDiffResultType)
}
func OpenRemoteGitHeadDiffResult(ch *Channel,frame []byte,id string)(string,error){
 return openRemoteBoundedText4096(ch,frame,id,remoteGitHeadDiffResultType)
}
func SealRemoteGitStageResult(ch *Channel,id string,content []byte)([]byte,error){
 // A Stage response contains only the committed Git blob object ID.
 if len(content)!=40&&len(content)!=64{return nil,ErrInvalidRemoteAction}
 for _,c:=range content {
  if !((c>='0'&&c<='9')||(c>='a'&&c<='f')){return nil,ErrInvalidRemoteAction}
 }
 return sealRemoteBoundedText4096(ch,id,content,remoteGitStageResultType)
}
func OpenRemoteGitStageResult(ch *Channel,frame []byte,id string)(string,error){
 result,err:=openRemoteBoundedText4096(ch,frame,id,remoteGitStageResultType)
 if err!=nil{return "",err}
 if len(result)!=40&&len(result)!=64{ch.Close();return "",ErrInvalidRemoteAction}
 for _,c:=range result{
  if !((c>='0'&&c<='9')||(c>='a'&&c<='f')){ch.Close();return "",ErrInvalidRemoteAction}
 }
 return result,nil
}
func SealRemoteGitUnstageResult(ch *Channel,id string,content []byte)([]byte,error){
 if string(content)!="unstaged"{return nil,ErrInvalidRemoteAction}
 return sealRemoteBoundedText4096(ch,id,content,remoteGitUnstageResultType)
}
func OpenRemoteGitUnstageResult(ch *Channel,frame []byte,id string)(string,error){
 result,err:=openRemoteBoundedText4096(ch,frame,id,remoteGitUnstageResultType)
 if err!=nil{return "",err}
 if result!="unstaged"{ch.Close();return "",ErrInvalidRemoteAction}
 return result,nil
}
func SealRemoteTaskListResult(ch *Channel,id string,content []byte)([]byte,error){
 return sealRemoteBoundedText4096(ch,id,content,remoteTaskListResultType)
}
func OpenRemoteTaskListResult(ch *Channel,frame []byte,id string)(string,error){
 return openRemoteBoundedText4096(ch,frame,id,remoteTaskListResultType)
}
func SealRemoteTaskRunTextResult(ch *Channel,id string,content []byte)([]byte,error){
 return sealRemoteBoundedText4096(ch,id,content,remoteTaskRunTextResultType)
}
func OpenRemoteTaskRunTextResult(ch *Channel,frame []byte,id string)(string,error){
 return openRemoteBoundedText4096(ch,frame,id,remoteTaskRunTextResultType)
}
func SealRemoteGitStatusFullResult(ch *Channel,id string,content []byte)([]byte,error){
 return sealRemoteBoundedText4096(ch,id,content,remoteGitStatusFullResultType)
}
func OpenRemoteGitStatusFullResult(ch *Channel,frame []byte,id string)(string,error){
 return openRemoteBoundedText4096(ch,frame,id,remoteGitStatusFullResultType)
}
func SealRemoteGitCommitReviewResult(ch *Channel,id string,content []byte)([]byte,error){
 return sealRemoteBoundedText4096(ch,id,content,remoteGitCommitReviewResultType)
}
func OpenRemoteGitCommitReviewResult(ch *Channel,frame []byte,id string)(string,error){
 return openRemoteBoundedText4096(ch,frame,id,remoteGitCommitReviewResultType)
}
func SealRemoteGitSwitchReviewResult(ch *Channel,id string,content []byte)([]byte,error){
 return sealRemoteBoundedText4096(ch,id,content,remoteGitSwitchReviewResultType)
}
func OpenRemoteGitSwitchReviewResult(ch *Channel,frame []byte,id string)(string,error){
 return openRemoteBoundedText4096(ch,frame,id,remoteGitSwitchReviewResultType)
}
func SealRemoteGitSwitchResult(ch *Channel,id string,content []byte)([]byte,error){
 if !validRemoteCommitObjectID(string(content)){return nil,ErrInvalidRemoteAction}
 return sealRemoteBoundedText4096(ch,id,content,remoteGitSwitchResultType)
}
func OpenRemoteGitSwitchResult(ch *Channel,frame []byte,id string)(string,error){
 result,err:=openRemoteBoundedText4096(ch,frame,id,remoteGitSwitchResultType)
 if err!=nil{return "",err}
 if !validRemoteCommitObjectID(result){ch.Close();return "",ErrInvalidRemoteAction}
 return result,nil
}
func SealRemoteGitBranchDeleteReviewResult(ch *Channel,id string,content []byte)([]byte,error){
 return sealRemoteBoundedText4096(ch,id,content,remoteGitBranchDeleteReviewResultType)
}
func OpenRemoteGitBranchDeleteReviewResult(ch *Channel,frame []byte,id string)(string,error){
 return openRemoteBoundedText4096(ch,frame,id,remoteGitBranchDeleteReviewResultType)
}
func SealRemoteGitBranchDeleteResult(ch *Channel,id string,content []byte)([]byte,error){
 if string(content)!="deleted"{return nil,ErrInvalidRemoteAction}
 return sealRemoteBoundedText4096(ch,id,content,remoteGitBranchDeleteResultType)
}
func OpenRemoteGitBranchDeleteResult(ch *Channel,frame []byte,id string)(string,error){
 value,err:=openRemoteBoundedText4096(ch,frame,id,remoteGitBranchDeleteResultType)
 if err!=nil{return "",err}
 if value!="deleted"{ch.Close();return "",ErrInvalidRemoteAction}
 return value,nil
}

func SealRemoteGitBranchReviewResult(ch *Channel,id string,content []byte)([]byte,error){
 return sealRemoteBoundedText4096(ch,id,content,remoteGitBranchReviewResultType)
}
func OpenRemoteGitBranchReviewResult(ch *Channel,frame []byte,id string)(string,error){
 return openRemoteBoundedText4096(ch,frame,id,remoteGitBranchReviewResultType)
}
func SealRemoteGitBranchCreateResult(ch *Channel,id string,content []byte)([]byte,error){
 if !validRemoteCommitObjectID(string(content)){return nil,ErrInvalidRemoteAction}
 return sealRemoteBoundedText4096(ch,id,content,remoteGitBranchCreateResultType)
}
func OpenRemoteGitBranchCreateResult(ch *Channel,frame []byte,id string)(string,error){
 text,err:=openRemoteBoundedText4096(ch,frame,id,remoteGitBranchCreateResultType)
 if err!=nil{return "",err}
 if !validRemoteCommitObjectID(text){ch.Close();return "",ErrInvalidRemoteAction}
 return text,nil
}
func SealRemoteGitCommitResult(ch *Channel,id string,content []byte)([]byte,error){
 if !validRemoteCommitObjectID(string(content)){return nil,ErrInvalidRemoteAction}
 return sealRemoteBoundedText4096(ch,id,content,remoteGitCommitResultType)
}
func OpenRemoteGitCommitResult(ch *Channel,frame []byte,id string)(string,error){
 value,err:=openRemoteBoundedText4096(ch,frame,id,remoteGitCommitResultType)
 if err!=nil{return "",err}
 if !validRemoteCommitObjectID(value){ch.Close();return "",ErrInvalidRemoteAction}
 return value,nil
}
func validRemoteCommitObjectID(v string)bool{
 if len(v)!=40&&len(v)!=64{return false}
 for _,c:=range v{
  if !((c>='0'&&c<='9')||(c>='a'&&c<='f')){return false}
 }
 return true
}
func validRemoteBoundedTextType(typ string)bool{
 return typ==remoteGitDiffResultType||typ==remoteFilePreviewResultType||typ==remoteFileLinesResultType||typ==remoteFindLinesResultType||
  typ==remoteGitLogResultType||typ==remoteGitBranchesResultType||typ==remoteGitRemotesResultType||typ==remoteGitTrackingRefsResultType||typ==remoteGitPullReviewResultType||typ==remoteGitPullChangesResultType||typ==remoteGitPullCachedReviewResultType||typ==remoteGitPullApplyCachedResultType||typ==remoteGitPullApplyFilesResultType||typ==remoteGitFetchReviewResultType||typ==remoteGitFetchApplyResultType||
  typ==remoteGitStagedDiffResultType||typ==remoteGitHeadDiffResultType||typ==remoteGitStageResultType||typ==remoteGitUnstageResultType||typ==remoteTaskListResultType||
  typ==remoteTaskRunTextResultType||typ==remoteGitStatusFullResultType||typ==remoteGitCommitReviewResultType||typ==remoteGitCommitResultType||typ==remoteGitBranchReviewResultType||typ==remoteGitBranchCreateResultType||typ==remoteGitSwitchReviewResultType||typ==remoteGitSwitchResultType||typ==remoteGitBranchDeleteReviewResultType||typ==remoteGitBranchDeleteResultType
}

func sealRemoteBoundedText4096(ch *Channel,id string,content []byte,typ string)([]byte,error){
 if ch==nil||!ch.Established()||!validActionID(id)||
  !validRemoteBoundedTextType(typ)||
  len(content)>RemoteGitDiffMaxBytes||!utf8.Valid(content){
  return nil,ErrInvalidRemoteAction
 }
 sum:=sha256.Sum256(content)
 clear,err:=json.Marshal(remoteFileReadResultWire{
  Type:typ,RequestID:id,OK:true,
  RemoteControlAvailable:false,Content:string(content),
  SHA256:hex.EncodeToString(sum[:]),
 })
 if err!=nil||len(clear)>RemoteGitDiffMaxPlaintext{return nil,ErrInvalidRemoteAction}
 frame,err:=ch.Seal(clear)
 if err!=nil||len(frame)>MaxEncryptedGitDiffFrame{return nil,ErrInvalidRemoteAction}
 return frame,nil
}

func openRemoteBoundedText4096(ch *Channel,frame []byte,id,expectedType string)(string,error){
 if ch==nil||!ch.Established()||!validActionID(id)||
  !validRemoteBoundedTextType(expectedType)||
  len(frame)<25||len(frame)>MaxEncryptedGitDiffFrame{return "",ErrInvalidRemoteAction}
 clear,err:=ch.Open(frame)
 if err!=nil{return "",ErrInvalidRemoteAction}
 defer wipeLoginBytes(clear)
 raw,err:=decodeActionObjectBounded(clear,RemoteGitDiffMaxPlaintext,
  "type","request_id","ok","remote_control_available","content","sha256")
 if err!=nil{ch.Close();return "",ErrInvalidRemoteAction}
 var typ,receivedID,content,hash string
 var ok,available bool
 if json.Unmarshal(raw["type"],&typ)!=nil||
  json.Unmarshal(raw["request_id"],&receivedID)!=nil||
  json.Unmarshal(raw["content"],&content)!=nil||
  json.Unmarshal(raw["sha256"],&hash)!=nil||
  json.Unmarshal(raw["ok"],&ok)!=nil||
  json.Unmarshal(raw["remote_control_available"],&available)!=nil||
  typ!=expectedType||receivedID!=id||!ok||available||
  len(content)>RemoteGitDiffMaxBytes||!utf8.ValidString(content){
  ch.Close();return "",ErrInvalidRemoteAction
 }
 sum:=sha256.Sum256([]byte(content))
 if hash!=hex.EncodeToString(sum[:]){ch.Close();return "",ErrInvalidRemoteAction}
 return content,nil
}
