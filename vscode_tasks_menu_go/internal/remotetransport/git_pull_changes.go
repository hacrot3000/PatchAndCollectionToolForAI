package remotetransport

import (
 "strconv"
 "strings"
 "unicode/utf8"
)

// A distinct authenticated E2EE domain for the bounded, read-only list
// of files an ALREADY FETCHED fast-forward would change. This is not an
// apply token and never authorizes a checkout, fetch, shell or ref update.
const remoteGitPullChangesResultType="taskdeck-e2ee-git-pull-changes-result-v1"
const RemoteGitPullChangesMaxFiles=32

// A deliberately narrow ASCII path language prevents terminal control,
// newline, escape, traversal and hidden metadata from becoming a claimed
// complete preview. Unsupported names fail the WHOLE preview closed.
func ValidRemotePullChangedPath(path string)bool{
 if len(path)<1||len(path)>180||strings.Contains(path,".."){return false}
 parts:=strings.Split(path,"/")
 if len(parts)>8{return false}
 for _,part:=range parts{
  if len(part)<1||len(part)>64||part[0]=='.'||part[len(part)-1]=='.'{return false}
  for i:=0;i<len(part);i++{
   c:=part[i]
   if (c>='a'&&c<='z')||(c>='A'&&c<='Z')||
    (c>='0'&&c<='9')||c=='_'||c=='-'||c=='.'{continue}
   return false
  }
 }
 return true
}

func ValidRemoteGitPullChangesReport(value string)bool{
 if len(value)>RemoteGitDiffMaxBytes||!utf8.ValidString(value){return false}
 lines:=strings.Split(value,"\n")
 if len(lines)<10||lines[0]!="GIT_PULL_CHANGES_V1"||
  lines[6]!="NETWORK none"||lines[7]!="-- FILE CHANGES --"||
  lines[len(lines)-1]!=""{return false}
 for _,prefix:=range []struct{n int;label string}{
  {1,"BRANCH "},{2,"HEAD "},{3,"UPSTREAM "},{4,"UPSTREAM_HEAD "},{5,"FILES "},
 }{
  if !strings.HasPrefix(lines[prefix.n],prefix.label){return false}
 }
 branch:=strings.TrimPrefix(lines[1],"BRANCH ")
 old:=strings.TrimPrefix(lines[2],"HEAD ")
 upstream:=strings.TrimPrefix(lines[3],"UPSTREAM ")
 target:=strings.TrimPrefix(lines[4],"UPSTREAM_HEAD ")
 if !validRemotePullRef(branch,"refs/heads/")||
  !validRemotePullRef(upstream,"refs/remotes/")||
  !validRemoteCommitObjectID(old)||!validRemoteCommitObjectID(target)||
  len(old)!=len(target)||old==target{return false}
 countText:=strings.TrimPrefix(lines[5],"FILES ")
 count,err:=strconv.Atoi(countText)
 if err!=nil||count<1||count>RemoteGitPullChangesMaxFiles||
  strconv.Itoa(count)!=countText||len(lines)!=9+count{return false}
 previous:=""
 for _,line:=range lines[8:8+count]{
  if len(line)<3||line[1]!=' '||
   (line[0]!='A'&&line[0]!='M'&&line[0]!='D'){return false}
  path:=line[2:]
  if !ValidRemotePullChangedPath(path)||path<=previous{return false}
  previous=path
 }
 return true
}

func SealRemoteGitPullChangesResult(ch *Channel,id string,content []byte)([]byte,error){
 if !ValidRemoteGitPullChangesReport(string(content)){return nil,ErrInvalidRemoteAction}
 return sealRemoteBoundedText4096(ch,id,content,remoteGitPullChangesResultType)
}
func OpenRemoteGitPullChangesResult(ch *Channel,frame []byte,id string)(string,error){
 value,err:=openRemoteBoundedText4096(ch,frame,id,remoteGitPullChangesResultType)
 if err!=nil{return "",err}
 if !ValidRemoteGitPullChangesReport(value){
  ch.Close()
  return "",ErrInvalidRemoteAction
 }
 return value,nil
}
