package remotetransport

import (
 "strings"
 "unicode"
 "unicode/utf8"
)

// A file selection is a one-based index into an authenticated Git Status
// snapshot. It is NOT a path; the Agent re-reads and compares the exact
// snapshot before deriving a restricted tracked filename itself.
const RemoteActionGitStageFile="git.stage_file"
const RemoteActionGitUnstageFile="git.unstage_file"
const MaxRemoteGitFileRows=12

func RemoteGitFileOperation(op string)bool{
 return op==RemoteActionGitStageFile||op==RemoteActionGitUnstageFile
}

func ValidRemoteGitFileSelection(index int,snapshot string)bool{
 if index<1||index>MaxRemoteGitFileRows||
  len(snapshot)<8||len(snapshot)>MaxRemoteGitStatusText||
  !utf8.ValidString(snapshot)||!strings.HasPrefix(snapshot,"## ")||
  strings.ContainsRune(snapshot,0){return false}
 lines:=strings.Split(snapshot,"\n")
 if len(lines)<index+1||len(lines)<2||lines[0]==""{return false}
 for _,r:=range snapshot{
  if r=='\n'||r=='\t'{continue}
  if unicode.IsControl(r)||unicode.In(r,unicode.Cf,unicode.Zl,unicode.Zp){
   return false
  }
 }
 return true
}
