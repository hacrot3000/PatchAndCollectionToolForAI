package remotetransport

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
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
  typ==remoteGitLogResultType||typ==remoteGitBranchesResultType||
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
