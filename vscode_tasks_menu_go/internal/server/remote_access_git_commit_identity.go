package server

import (
 "os"
 "regexp"
 "strings"
 "unicode"
 "unicode/utf8"
)

// Git Commit deliberately disables global Git config and inherited GIT_*
// author variables. Repositories that rely on ~/.gitconfig alone can
// instead supply this explicit pair in the Agent's LOCAL environment.
// Never accept it from Portal/E2EE messages, nor echo it into an audit row.
var remoteGitCommitAuthorEmailPattern=regexp.MustCompile(
 `^[A-Za-z0-9][A-Za-z0-9._%+-]{0,63}@[A-Za-z0-9][A-Za-z0-9.-]{0,125}$`,
)

func remoteGitCommitHostIdentity()(name,email string,explicit,valid bool){
 name=os.Getenv("TASKDECK_REMOTE_GIT_AUTHOR_NAME")
 email=os.Getenv("TASKDECK_REMOTE_GIT_AUTHOR_EMAIL")
 if name==""&&email==""{return "","",false,true}
 if name==""||email==""||len(name)>96||len(email)>192||
  !utf8.ValidString(name)||strings.TrimSpace(name)!=name||
  !remoteGitCommitAuthorEmailPattern.MatchString(email){
  return "","",false,false
 }
 for _,r:=range name{
  if !(unicode.IsLetter(r)||unicode.IsDigit(r)||
   r==' '||r=='-'||r=='_'||r=='.'||r=='\''){
   return "","",false,false
  }
 }
 domain:=strings.SplitN(email,"@",2)[1]
 for _,part:=range strings.Split(domain,"."){
  if part==""||strings.HasPrefix(part,"-")||strings.HasSuffix(part,"-"){
   return "","",false,false
  }
 }
 return name,email,true,true
}
