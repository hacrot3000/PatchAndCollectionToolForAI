package main

import (
 "bufio"
 "bytes"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "errors"
 "strings"
 "strconv"
 "testing"
)

type fakeInteractiveTaskDeck struct{
 actions []string
 replacePath,replaceText,replaceSHA string
 previewText string
 linePageVersions []string
 linePageCalls int
 linePageTotal int64
 taskCatalogue string
 commitReview,commitMessage string
 branchReview,branchSlug string
 switchReview,switchSlug string
 deleteReview,deleteSlug string
 unstagedDiff,stagedDiff string
 denyStaged bool
 emptyStaged bool
 denied bool
}
func (f *fakeInteractiveTaskDeck) record(action string)(string,error){
 f.actions=append(f.actions,action)
 if f.denied{return "",errors.New("TaskDeck revoked authenticated session")}
 return "ok",nil
}
func (f *fakeInteractiveTaskDeck) ReadFile(_ context.Context,p string)(string,error){
 f.actions=append(f.actions,"view:"+p)
 if f.denied{return "",errors.New("revoked")}
 return "before",nil
}
func (f *fakeInteractiveTaskDeck) FindFileLines(_ context.Context,path,term string)(string,error){
 f.actions=append(f.actions,"find-lines:"+path+":"+term)
 if f.denied{return "",errors.New("revoked")}
 sha:=strings.Repeat("a",64)
 if len(f.linePageVersions)>0{
  i:=f.linePageCalls
  if i>=len(f.linePageVersions){i=len(f.linePageVersions)-1}
  sha=f.linePageVersions[i]
 }
 f.linePageCalls++
 termSHA:=sha256.Sum256([]byte(term))
 return "FILE_SHA256 "+sha+
  "\nTERM_SHA256 "+hex.EncodeToString(termSHA[:])+
  "\nTOTAL_LINES 1\nMATCHES 1\n-- COMPLETE MATCHING LINES --\n1: "+term+"\n",nil
}
func (f *fakeInteractiveTaskDeck) InsertLargeFileLine(_ context.Context,path string,line int64,content,sha string)(string,error){
 f.replacePath=path
 f.replaceText=content
 f.replaceSHA=sha
 return f.record("insert-large-line:"+path+":"+strconv.FormatInt(line,10))
}
func (f *fakeInteractiveTaskDeck) DeleteLargeFileLine(_ context.Context,path string,line int64,sha string)(string,error){
 f.replacePath=path
 f.replaceSHA=sha
 return f.record("delete-large-line:"+path+":"+strconv.FormatInt(line,10))
}
func (f *fakeInteractiveTaskDeck) EditLargeFileLine(_ context.Context,path string,line int64,content,sha string)(string,error){
 f.replacePath=path
 f.replaceText=content
 f.replaceSHA=sha
 return f.record("edit-large-line:"+path+":"+strconv.FormatInt(line,10))
}
func (f *fakeInteractiveTaskDeck) ReadFileLines(_ context.Context,p string,start int64)(string,error){
 f.actions=append(f.actions,"view-lines:"+p+":"+strconv.FormatInt(start,10))
 if f.denied{return "",errors.New("revoked")}
 var hash string
 if len(f.linePageVersions)>0 {
  i:=f.linePageCalls
  if i>=len(f.linePageVersions){i=len(f.linePageVersions)-1}
  hash=f.linePageVersions[i]
 } else {hash=strings.Repeat("a",64)}
 f.linePageCalls++
 total:=start
 var next int64
 if f.linePageTotal>start {
  total=f.linePageTotal
  next=start+1
 }
 return "FILE_SHA256 "+hash+"\nSTART_LINE "+strconv.FormatInt(start,10)+
  "\nNEXT_LINE "+strconv.FormatInt(next,10)+"\nTOTAL_LINES "+strconv.FormatInt(total,10)+
  "\n-- COMPLETE SOURCE LINES --\n"+
  strconv.FormatInt(start,10)+": example\n",nil
}
func (f *fakeInteractiveTaskDeck) ReadFilePreview(_ context.Context,p string)(string,error){
 f.actions=append(f.actions,"preview:"+p)
 if f.denied{return "",errors.New("revoked")}
 if f.previewText!=""{return f.previewText,nil}
 return "preview",nil
}
func (f *fakeInteractiveTaskDeck) ListDirectory(_ context.Context,p string)(string,error){
 return f.record("ls:"+p)
}
func (f *fakeInteractiveTaskDeck) GitStatus(_ context.Context)(string,error){
 return f.record("git:status")
}
func (f *fakeInteractiveTaskDeck) GitSwitchReview(_ context.Context,slug string)(string,error){
 _,err:=f.record("git:switch-review:"+slug)
 if err!=nil{return "",err}
 if f.switchReview!=""{return f.switchReview,nil}
 oid:=strings.Repeat("a",40)
 return "HEAD "+oid+"\nBRANCH refs/heads/main\nTARGET refs/heads/taskdeck/"+slug+
  "\nTARGET_HEAD "+oid+"\nINDEX_SHA256 "+strings.Repeat("b",64)+
  "\nCLEAN yes\n",nil
}
func (f *fakeInteractiveTaskDeck) GitSwitchSameHead(_ context.Context,slug,sha string)(string,error){
 f.switchSlug=slug
 f.replaceSHA=sha
 return f.record("git:switch")
}
func (f *fakeInteractiveTaskDeck) GitBranchDeleteReview(_ context.Context,slug string)(string,error){
 _,err:=f.record("git:delete-review:"+slug)
 if err!=nil{return "",err}
 if f.deleteReview!=""{return f.deleteReview,nil}
 oid:=strings.Repeat("a",40)
 return "HEAD "+oid+"\nBRANCH refs/heads/main\nTARGET refs/heads/taskdeck/"+slug+
  "\nTARGET_HEAD "+oid+"\nINDEX_SHA256 "+strings.Repeat("b",64)+"\nCLEAN yes\n",nil
}
func (f *fakeInteractiveTaskDeck) GitBranchDelete(_ context.Context,slug,sha string)(string,error){
 f.deleteSlug=slug
 f.replaceSHA=sha
 return f.record("git:branch-delete")
}
func (f *fakeInteractiveTaskDeck) GitBranchReview(_ context.Context)(string,error){
 _,err:=f.record("git:branch-review")
 if err!=nil{return "",err}
 if f.branchReview!=""{return f.branchReview,nil}
 return "HEAD "+strings.Repeat("a",40)+"\nBRANCH refs/heads/main\n",nil
}
func (f *fakeInteractiveTaskDeck) GitBranchCreate(_ context.Context,slug,sha string)(string,error){
 f.branchSlug=slug
 f.replaceSHA=sha
 return f.record("git:branch-create")
}
func (f *fakeInteractiveTaskDeck) GitCommitReview(_ context.Context)(string,error){
 value,err:=f.record("git:commit-review")
 if err!=nil{return "",err}
 if f.commitReview!=""{return f.commitReview,nil}
 return value,nil
}
func (f *fakeInteractiveTaskDeck) GitCommit(_ context.Context,message,sha string)(string,error){
 f.commitMessage=message
 f.replaceSHA=sha
 return f.record("git:commit")
}
func (f *fakeInteractiveTaskDeck) GitStatusFull(_ context.Context)(string,error){
 return f.record("git:status")
}
func (f *fakeInteractiveTaskDeck) GitLog(_ context.Context)(string,error){
 return f.record("git:log")
}
func (f *fakeInteractiveTaskDeck) GitBranches(_ context.Context)(string,error){
 return f.record("git:branches")
}
func (f *fakeInteractiveTaskDeck) GitDiff(_ context.Context,p string)(string,error){
 result,err:=f.record("git:diff:"+p)
 if err!=nil{return "",err}
 if f.unstagedDiff!=""{return f.unstagedDiff,nil}
 return result,nil
}
func (f *fakeInteractiveTaskDeck) ReplaceFile(_ context.Context,p,text,sha string)(string,error){
 f.replacePath=p;f.replaceText=text;f.replaceSHA=sha
 return f.record("edit:"+p)
}
func (f *fakeInteractiveTaskDeck) ReplaceFileText(_ context.Context,p,text,sha string)(string,error){
 f.replacePath=p;f.replaceText=text;f.replaceSHA=sha
 return f.record("edit:"+p)
}
func (f *fakeInteractiveTaskDeck) ListApprovedTasks(_ context.Context)(string,error){
 _,err:=f.record("task:list")
 if err!=nil{return "",err}
 if f.taskCatalogue!=""{return f.taskCatalogue,nil}
 return "1\tApproved host task\n",nil
}
func (f *fakeInteractiveTaskDeck) GitUnstage(_ context.Context,path,sha string)(string,error){
 f.replaceSHA=sha
 return f.record("git:unstage:"+path)
}
func (f *fakeInteractiveTaskDeck) GitStage(_ context.Context,path,sha string)(string,error){
 f.replaceSHA=sha
 return f.record("git:stage:"+path)
}
func (f *fakeInteractiveTaskDeck) GitHeadDiff(_ context.Context,path string)(string,error){
 return f.record("git:head:"+path)
}
func (f *fakeInteractiveTaskDeck) GitStagedDiff(_ context.Context,path string)(string,error){
 result,err:=f.record("git:staged:"+path)
 if err!=nil{return "",err}
 if f.denyStaged{return "",errors.New("revoked between Git reads")}
 if f.emptyStaged{return "",nil}
 if f.stagedDiff!=""{return f.stagedDiff,nil}
 return result,nil
}
func (f *fakeInteractiveTaskDeck) StartManagedTask(_ context.Context,id int)(string,error){
 return f.record("managed:start:"+strconv.Itoa(id))
}
func (f *fakeInteractiveTaskDeck) ReadManagedOutput(_ context.Context,handle string,offset int64)(string,error){
 return f.record("managed:output:"+handle+":"+strconv.FormatInt(offset,10))
}
func (f *fakeInteractiveTaskDeck) StatusManagedTask(_ context.Context,handle string)(string,error){
 return f.record("managed:status:"+handle)
}
func (f *fakeInteractiveTaskDeck) CancelManagedTask(_ context.Context,handle string)(string,error){
 return f.record("managed:cancel:"+handle)
}
func (f *fakeInteractiveTaskDeck) RunTaskReport(_ context.Context,id int)(string,error){
 return f.record("task-report:"+strconv.Itoa(id))
}
func (f *fakeInteractiveTaskDeck) RunTaskText(_ context.Context,id int)(string,error){
 return f.record("task:"+string(rune(id+'0')))
}
func (f *fakeInteractiveTaskDeck) RunTask(_ context.Context,id int)(string,error){
 return f.record("task:"+string(rune(id+'0')))
}

func TestInteractiveNativeConsoleRoutesOnlyTypedE2EEOperations(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 input:="ls\nview a.txt\ngit status\ngit diff a.txt\nterminal task 1\nRUN TASK 1\nexit\n"
 var output bytes.Buffer
 err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(input)),&output)
 if err!=nil{t.Fatal(err)}
 got:=strings.Join(f.actions,",")
 if got!="ls:.,preview:a.txt,git:status,git:diff:a.txt,task:list,task:1"{
  t.Fatalf("wrong typed actions %q",got)
 }
 if !strings.Contains(output.String(),"Terminal PTY/shell") {
  t.Fatal("console misrepresents itself as generic terminal")
 }
}
func TestInteractiveNativeConsoleEditUsesLocalSHAAndExplicitYES(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var output bytes.Buffer
 input:="edit a.txt\nafter\n.save-noeol\nNO\nedit a.txt\nafter\n.save-noeol\nYES\nexit\n"
 err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(input)),&output)
 if err!=nil{t.Fatal(err)}
 sum:=sha256.Sum256([]byte("preview"))
 if f.replacePath!="a.txt"||f.replaceText!="after"||
  f.replaceSHA!=hex.EncodeToString(sum[:]){
  t.Fatalf("edit data or optimistic SHA did not derive from authenticated view: path=%q data=%q sha=%s",f.replacePath,f.replaceText,f.replaceSHA)
 }
 if len(f.actions)!=3{
  t.Fatalf("cancel must not issue a file write: %+v",f.actions)
 }
}
func TestInteractiveConsoleRejectsShellGitFlagsTraversalAndStopsOnRevoke(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 script:="terminal sh -c id\ngit push origin main\ngit diff ../private\nview /etc/passwd\nedit ../escape\nterminal task -1\nls\nexit\n"
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(script)),&out);err!=nil{t.Fatal(err)}
 if len(f.actions)!=1||f.actions[0]!="ls:."{
  t.Fatalf("untrusted command was dispatched: %+v",f.actions)
 }
 f=&fakeInteractiveTaskDeck{denied:true}
 err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader("git status\nls\n")),&out)
 if err==nil||len(f.actions)!=1{
  t.Fatal("revoked principal continued acting in a poisoned E2EE session")
 }
}
func TestInteractiveConsoleDoesNotBypassPerLoginSixActionQuota(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(strings.Repeat("git status\n",10))),&out)
 if err!=nil||len(f.actions)!=6{
  t.Fatalf("native console bypassed Relay quota: calls=%d err=%v",len(f.actions),err)
 }
}


func TestInteractiveConsoleWorkspaceNavigationAndSpaces(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 script:="cd folder with spaces\npwd\nls\nview file with spaces.txt\ngit diff change with spaces.go\ncd ..\nls\nexit\n"
 err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(script)),&out)
 if err!=nil{t.Fatal(err)}
 want:="ls:folder with spaces,preview:folder with spaces/file with spaces.txt,git:diff:folder with spaces/change with spaces.go,ls:."
 if got:=strings.Join(f.actions,",");got!=want{t.Fatalf("unexpected rooted commands %q, want %q",got,want)}
 if !strings.Contains(out.String(),"folder with spaces"){
  t.Fatal("client-only cwd not displayed")
 }
}

func TestInteractiveConsoleNavigationNeverEscapesProjectRoot(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 script:="cd ..\ncd ../escape\ncd nested\ncd ../../../../etc\nview ../../secret\ngit diff /etc/passwd\nterminal task 1;echo\nls\nexit\n"
 err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(script)),&out)
 if err!=nil{t.Fatal(err)}
 if got:=strings.Join(f.actions,",");got!="ls:nested"{
  t.Fatalf("client path traversal or command injection was forwarded: %q",got)
 }
 if !strings.Contains(out.String(),"workspace root"){
  t.Fatal("missing local root-escape rejection")
 }
}

func TestInteractiveConsoleBoundsTTYLinesBeforeRemoteDispatch(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader("view "+strings.Repeat("x",300)+"\nls\n")),&out)
 if err==nil||len(f.actions)!=0{t.Fatalf("oversized TTY input reached remote: %v %+v",err,f.actions)}
 f=&fakeInteractiveTaskDeck{}
 err=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader("edit a.txt\n"+strings.Repeat("y",5000)+"\nYES\n")),&out)
 if err==nil||len(f.actions)!=1||f.replacePath!=""{
  t.Fatalf("oversized edit buffer caused a remote write: %v %+v",err,f.actions)
 }
}

func TestInteractiveConsoleCleanExitAtEOF(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader("pwd\nexit")),&out)
 if err!=nil||len(f.actions)!=0{t.Fatalf("EOF exit must be local: %v %+v",err,f.actions)}
}

func TestInteractiveGitLogAndBranchesUseOnlyFixedTypedOperations(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 script:="git log\ngit branches\ngit log --oneline\ngit push\nexit\n"
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(script)),&out);err!=nil{t.Fatal(err)}
 if got:=strings.Join(f.actions,",");got!="git:log,git:branches"{
  t.Fatalf("untyped Git arguments reached remote host: %q",got)
 }
}

func TestInteractiveMultilineEditPreservesNewlinesAndRequiresApproval(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 input:="edit dir/a.txt\nfirst\nsecond\n.save\nYES\nexit\n"
 err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(input)),&out)
 if err!=nil{t.Fatal(err)}
 if f.replacePath!="dir/a.txt"||f.replaceText!="first\nsecond\n"{
  t.Fatalf("multiline content malformed: %q %q",f.replacePath,f.replaceText)
 }
 if len(f.actions)!=2||f.actions[0]!="preview:dir/a.txt"||f.actions[1]!="edit:dir/a.txt"{
  t.Fatalf("edited without verified preview or after unauthorized action: %+v",f.actions)
 }
}
func TestInteractiveEditCancelNeverWrites(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 input:="edit a.txt\nsensitive draft\n.cancel\nexit\n"
 err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(input)),&out)
 if err!=nil||f.replacePath!=""||len(f.actions)!=1{
  t.Fatalf("canceled edit wrote: %v %+v",err,f.actions)
 }
}

func TestInteractiveRemoteTaskCatalogueAndStagedDiffTypedOnly(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 script:="terminal tasks\ngit diff --staged src/file name.go\nterminal task 1\nRUN TASK 1\n"+
  "terminal tasks --all\ngit diff --cached src/file name.go\nterminal sh -c id\nexit\n"
 err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(script)),&out)
 if err!=nil{t.Fatal(err)}
 if got:=strings.Join(f.actions,",");got!="task:list,git:staged:src/file name.go,task:list,task:1"{
  t.Fatalf("native console forwarded untyped Terminal/Git command: %q",got)
 }
}

func TestInteractiveReplaceUniqueMatchUsesAuthenticatedCAS(t *testing.T){
 f:=&fakeInteractiveTaskDeck{previewText:"first\nkey=old\nlast\n"}
 var out bytes.Buffer
 script:="replace settings/app config.txt\nkey=old\nkey=new\nYES\nexit\n"
 err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(script)),&out)
 if err!=nil{t.Fatal(err)}
 digest:=sha256.Sum256([]byte(f.previewText))
 if f.replacePath!="settings/app config.txt"||
  f.replaceText!="first\nkey=new\nlast\n"||
  f.replaceSHA!=hex.EncodeToString(digest[:]){
  t.Fatalf("replace did not pin full authenticated preview and CAS: %+v",f)
 }
 if got:=strings.Join(f.actions,",");got!="preview:settings/app config.txt,edit:settings/app config.txt"{
  t.Fatalf("unexpected remote calls: %s",got)
 }
 if !strings.Contains(out.String(),"first\nkey=new\nlast\n"){
  t.Fatal("preview is no longer readable with real line breaks")
 }
}

func TestInteractiveReplaceRejectsMissingAndAmbiguousMatchWithoutWrite(t *testing.T){
 for _,tc:=range []struct{content,search string}{
  {"abc abc","abc"},
  {"abcdef","missing"},
 }{
  f:=&fakeInteractiveTaskDeck{previewText:tc.content}
  var out bytes.Buffer
  script:="replace a.txt\n"+tc.search+"\nnew\nexit\n"
  err:=runNativeInteractiveConsole(context.Background(),f,
   bufio.NewReader(strings.NewReader(script)),&out)
  if err!=nil{t.Fatal(err)}
  if len(f.actions)!=1||f.replacePath!=""{
   t.Fatalf("ambiguous/missing search caused a write: %+v",f)
  }
 }
}

func TestInteractiveReplaceCancellationAndOversizeDoNotWrite(t *testing.T){
 f:=&fakeInteractiveTaskDeck{previewText:"key"}
 var out bytes.Buffer
 script:="replace a.txt\nkey\nnew\nNO\nexit\n"
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(script)),&out);err!=nil{t.Fatal(err)}
 if len(f.actions)!=1||f.replacePath!=""{t.Fatalf("NO did not cancel: %+v",f)}
 f=&fakeInteractiveTaskDeck{previewText:"key"+strings.Repeat("z",4090)}
 out.Reset()
 script="replace a.txt\nkey\n"+strings.Repeat("x",10)+"\nexit\n"
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(script)),&out);err!=nil{t.Fatal(err)}
 if len(f.actions)!=1||f.replacePath!=""{t.Fatalf("oversized edit wrote: %+v",f)}
}

func TestInteractiveConsoleEscapesTerminalControlsButPreservesNewlines(t *testing.T){
 var out bytes.Buffer
 consoleDisplayText(&out,"one\n\033[31mred\r\n\u202eRIGHT\u2067X\u2069")
 got:=out.String()
 if strings.ContainsRune(got,'\x1b')||strings.ContainsRune(got,'\r')||
  strings.ContainsRune(got,'\u202e')||strings.ContainsRune(got,'\u2067'){
  t.Fatalf("terminal control escaped incorrectly: %q",got)
 }
 if !strings.Contains(got,"one\n")||!strings.Contains(got,"\\u001b")||
  !strings.Contains(got,"\\u000d")||!strings.Contains(got,"\\u202e"){
  t.Fatalf("remote text presentation lost line boundaries or sanitizer: %q",got)
 }
}

func TestInteractiveReplaceRespectsSessionQuotaAndRejectsTraversal(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 script:=strings.Repeat("git status\n",5)+"replace a.txt\npwd\nexit\n"
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(script)),&out);err!=nil{t.Fatal(err)}
 if len(f.actions)!=5||f.replacePath!=""{
  t.Fatalf("replace issued unsafe first leg at quota boundary: %+v",f)
 }
 f=&fakeInteractiveTaskDeck{}
 out.Reset()
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader("replace ../../etc/passwd\nexit\n")),&out);err!=nil{t.Fatal(err)}
 if len(f.actions)!=0{t.Fatalf("traversal reached Agent: %+v",f.actions)}
}


func TestInteractiveFindReadsOnceAndPrintsMatchingLineNumbers(t *testing.T){
 f:=&fakeInteractiveTaskDeck{previewText:"target\nordinary\ntarget and \x1b[31mcolor\n"}
 var out bytes.Buffer
 script:="find folder with spaces/log.txt\ntarget\nexit\n"
 err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(script)),&out)
 if err!=nil{t.Fatal(err)}
 if got:=strings.Join(f.actions,",");got!="preview:folder with spaces/log.txt"{
  t.Fatalf("find bypassed authenticated preview boundary: %s",got)
 }
 if !strings.Contains(out.String(),"2 vị trí")||
  !strings.Contains(out.String(),"1: target\n")||
  !strings.Contains(out.String(),"3: target and \\u001b[31mcolor\n")||
  strings.ContainsRune(out.String(),'\x1b'){
  t.Fatalf("find incorrectly rendered lines or untrusted terminal escapes: %q",out.String())
 }
}

func TestInteractiveFindInvalidInputOrMissingMatchNeverWrites(t *testing.T){
 for _,tc:=range []struct{term string;want string}{
  {"","không hợp lệ"},
  {"not found","0 vị trí"},
  {"\x1b[31m","không hợp lệ"},
  {"\u202eabc","không hợp lệ"},
 }{
  f:=&fakeInteractiveTaskDeck{previewText:"hello\n"}
  var out bytes.Buffer
  script:="find a.txt\n"+tc.term+"\nexit\n"
  err:=runNativeInteractiveConsole(context.Background(),f,
   bufio.NewReader(strings.NewReader(script)),&out)
  if err!=nil{t.Fatal(err)}
  if len(f.actions)!=1||f.actions[0]!="preview:a.txt"||
   f.replacePath!=""||!strings.Contains(out.String(),tc.want){
   t.Fatalf("find invalid/missing input was unsafe: %+v, output=%q",f,out.String())
  }
 }
}

func TestInteractiveReplaceAllRequiresExactCountAndFullSHA(t *testing.T){
 f:=&fakeInteractiveTaskDeck{previewText:"foo foobar\nfoo\n"}
 var out bytes.Buffer
 script:="replace-all settings/app config.txt\nfoo\nbar\nYES 3\nexit\n"
 err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(script)),&out)
 if err!=nil{t.Fatal(err)}
 sum:=sha256.Sum256([]byte(f.previewText))
 if f.replacePath!="settings/app config.txt"||
  f.replaceText!="bar barbar\nbar\n"||
  f.replaceSHA!=hex.EncodeToString(sum[:]){
  t.Fatalf("replace-all lost exact full-preview optimistic edit: %+v",f)
 }
 if got:=strings.Join(f.actions,",");got!="preview:settings/app config.txt,edit:settings/app config.txt"{
  t.Fatalf("unexpected E2EE actions: %s",got)
 }
 if !strings.Contains(out.String(),"3 vị trí")||
  !strings.Contains(out.String(),"YES 3")||
  !strings.Contains(out.String(),"bar barbar\nbar\n"){
  t.Fatalf("missing exact confirmation or full proposed content: %q",out.String())
 }
}

func TestInteractiveReplaceAllNoMatchWrongCountNoOpAndOversizeNeverWrite(t *testing.T){
 cases:=[]struct{content,search,replacement,confirm string}{
  {"foo foo","foo","bar","YES 1"},
  {"foo","missing","bar","YES 1"},
  {"foo","foo","foo","YES 1"},
  {"foo"+strings.Repeat("z",4088),"foo",strings.Repeat("x",30),"YES 1"},
  {"foo","foo","\x1b[31m","YES 1"},
 }
 for _,tc:=range cases{
  f:=&fakeInteractiveTaskDeck{previewText:tc.content}
  var out bytes.Buffer
  script:="replace-all a.txt\n"+tc.search+"\n"+tc.replacement+"\n"+tc.confirm+"\nexit\n"
  err:=runNativeInteractiveConsole(context.Background(),f,
   bufio.NewReader(strings.NewReader(script)),&out)
  if err!=nil{t.Fatal(err)}
  if f.replacePath!=""{
   t.Fatalf("unsafe replace-all write for %#v: %+v",tc,f)
  }
  if got:=strings.Join(f.actions,",");got!="preview:a.txt"{
   t.Fatalf("unexpected call despite refusal: %s",got)
  }
 }
}

func TestInteractiveReplaceAllStopsBeforePreviewWhenQuotaInsufficient(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 script:=strings.Repeat("git status\n",5)+"replace-all a.txt\nexit\n"
 err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(script)),&out)
 if err!=nil{t.Fatal(err)}
 if len(f.actions)!=5||f.replacePath!=""{
  t.Fatalf("replace-all ignored two-round quota: %+v",f)
 }
}

func TestInteractiveFindAndReplaceAllPreventTraversal(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 script:="find ../../etc/passwd\nreplace-all /tmp/pwn\nexit\n"
 err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(script)),&out)
 if err!=nil||len(f.actions)!=0{
  t.Fatalf("path traversal escaped local workspace boundary: %v %+v",err,f.actions)
 }
}


func TestInteractiveLiteralRejectsHiddenUnicodeAndControls(t *testing.T) {
 for _,term:=range []string{"\u200b","foo\u2028bar","foo\u2029bar","\u2066word\u2069","\x1b[31m"} {
  if validConsoleLiteral(term,false) {
   t.Fatalf("invisible/control literal accepted: %q",term)
  }
 }
 if !validConsoleLiteral("giá-trị mới",false)||!validConsoleLiteral("",true)||
  validConsoleLiteral("",false){
  t.Fatal("valid ordinary UTF-8 literal unexpectedly denied or empty accepted")
 }
 var out bytes.Buffer
 consoleDisplayText(&out,"left\u200bmiddle\u2028right\u2029end\n")
 if got:=out.String();!strings.Contains(got,"\\u200b")||
  !strings.Contains(got,"\\u2028")||!strings.Contains(got,"\\u2029")||
  strings.ContainsRune(got,'\u200b') {
  t.Fatalf("untrusted Unicode format escaped incorrectly: %q",got)
 }
}


func TestInteractiveConsoleRejectsTerminalControlCharactersInPaths(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 input:="view a\x1b[31m.txt\nfind a\u202eb.txt\nreplace-all a\u200bb.txt\nexit\n"
 err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(input)),&out)
 if err!=nil||len(f.actions)!=0{
  t.Fatalf("path terminal spoof forwarded to host: %v %+v",err,f.actions)
 }
}


func TestInteractiveLinesShowsExactNumberedContentWithoutTerminalEscape(t *testing.T) {
 f:=&fakeInteractiveTaskDeck{previewText:"a\r\n\x1b[31msecond\n\nlast"}
 var out bytes.Buffer
 script:="lines dir/log with spaces.txt\nexit\n"
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(script)),&out);err!=nil{t.Fatal(err)}
 if got:=strings.Join(f.actions,",");got!="preview:dir/log with spaces.txt"{
  t.Fatalf("lines issued unexpected E2EE actions: %s",got)
 }
 rendered:=out.String()
 if !strings.Contains(rendered,"1: a\n")||
  !strings.Contains(rendered,"2: \\u001b[31msecond\n")||
  !strings.Contains(rendered,"3: \n")||
  !strings.Contains(rendered,"4: last\n")||
  strings.ContainsRune(rendered,'\x1b')||strings.ContainsRune(rendered,'\r'){
  t.Fatalf("numbered remote content exposed ANSI or incorrect lines: %q",rendered)
 }
}

func TestInteractiveEditLinePreservesCRLFAndUsesCompleteFileSHA(t *testing.T){
 f:=&fakeInteractiveTaskDeck{previewText:"alpha\r\nold=value\r\nomega\n"}
 var out bytes.Buffer
 script:="edit-line folder/config name.ini\n2\nnew=value\nYES 2\nexit\n"
 err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(script)),&out)
 if err!=nil{t.Fatal(err)}
 sum:=sha256.Sum256([]byte(f.previewText))
 if f.replacePath!="folder/config name.ini"||
  f.replaceText!="alpha\r\nnew=value\r\nomega\n"||
  f.replaceSHA!=hex.EncodeToString(sum[:]){
  t.Fatalf("line edit failed complete-file optimistic CAS: %+v",f)
 }
 if got:=strings.Join(f.actions,",");got!="preview:folder/config name.ini,edit:folder/config name.ini"{
  t.Fatalf("edit-line unexpected action count: %s",got)
 }
 if !strings.Contains(out.String(),"YES 2"){
  t.Fatal("line edit missing explicit line-specific confirmation")
 }
}

func TestInteractiveConsoleReplaceLinePreservesAllOtherBytesAndEndings(t *testing.T) {
 cases:=[]struct{before string;index int;newValue,want string}{
  {"first\nlast",2,"changed","first\nchanged"},
  {"first\r\nlast\r\n",2,"changed","first\r\nchanged\r\n"},
  {"\nsecond\n",1,"added","added\nsecond\n"},
  {"a\n\nc",2,"b","a\nb\nc"},
  {"a\n",1,"b","b\n"},
 }
 for _,tc:=range cases {
  _,result,err:=consoleReplaceLine(tc.before,tc.index,tc.newValue)
  if err!=nil||result!=tc.want{
   t.Fatalf("line edit %q index=%d got %q err=%v want %q",
    tc.before,tc.index,result,err,tc.want)
  }
 }
}

func TestInteractiveConsoleReplaceLineRejectsNonexistentNoopControlAndOverflow(t *testing.T){
 cases:=[]struct{data string;index int;replacement string}{
  {"",1,"new"},
  {"one\n",2,"phantom"},
  {"one",0,"new"},
  {"one",2,"new"},
  {"same",1,"same"},
  {"a",1,"\x1b[31m"},
  {"a",1,"\u200bevil"},
  {"a"+strings.Repeat("z",4090),1,strings.Repeat("b",4097)},
 }
 for _,tc:=range cases{
  _,next,err:=consoleReplaceLine(tc.data,tc.index,tc.replacement)
  if err==nil||next!=""{
   t.Fatalf("invalid line edit accepted: index=%d err=%v",tc.index,err)
  }
 }
}

func TestInteractiveEditLineCancelAndOutOfRangeNeverWrite(t *testing.T){
 for _,script:=range []string{
  "edit-line a.txt\n1\nnew\nYES 2\nexit\n",
  "edit-line a.txt\n10\nnew\nYES 10\nexit\n",
  "edit-line a.txt\n0\nnew\nYES 0\nexit\n",
  "edit-line a.txt\n1\npreview\nYES 1\nexit\n",
 }{
  f:=&fakeInteractiveTaskDeck{}
  var out bytes.Buffer
  err:=runNativeInteractiveConsole(context.Background(),f,
   bufio.NewReader(strings.NewReader(script)),&out)
  if err!=nil{t.Fatal(err)}
  if f.replacePath!=""||len(f.actions)!=1||f.actions[0]!="preview:a.txt"{
   t.Fatalf("rejected line edit wrote to remote: %+v",f)
  }
 }
}

func TestInteractiveEditLineDoesNotStartWithoutTwoAvailableRounds(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 script:=strings.Repeat("git status\n",5)+"edit-line a.txt\nexit\n"
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(script)),&out);err!=nil{t.Fatal(err)}
 if len(f.actions)!=5||f.replacePath!=""{
  t.Fatalf("line edit ignored quota or attempted write: %+v",f)
 }
}

func TestInteractiveLinesAndEditLineRejectTraversalBeforeRead(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 script:="lines ../secret\nedit-line /etc/passwd\nexit\n"
 err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(script)),&out)
 if err!=nil||len(f.actions)!=0{
  t.Fatalf("untrusted line file path forwarded: %v %+v",err,f.actions)
 }
}


func TestInteractiveGitReviewUsesTwoSeparateReadOnlyHostGatedActions(t *testing.T) {
 f:=&fakeInteractiveTaskDeck{
  unstagedDiff:"unstaged -old +new\n",stagedDiff:"staged -before +after\n",
 }
 var out bytes.Buffer
 script:="cd dir with spaces\ngit review config with spaces.txt\nexit\n"
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(script)),&out);err!=nil{t.Fatal(err)}
 if got:=strings.Join(f.actions,",");got!="git:diff:dir with spaces/config with spaces.txt,git:staged:dir with spaces/config with spaces.txt" {
  t.Fatalf("git review bypassed exact typed read-only Git operations: %s",got)
 }
 display:=out.String()
 if !strings.Contains(display,"-- Chưa stage")||!strings.Contains(display,"unstaged -old +new")||
  !strings.Contains(display,"-- Đã stage")||!strings.Contains(display,"staged -before +after")||
  !strings.Contains(display,"Git review chỉ đọc") {
  t.Fatalf("missing separately labeled staged and unstaged review: %q",display)
 }
}

func TestInteractiveGitReviewRespectsQuotaBeforeEitherRead(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 script:=strings.Repeat("git status\n",5)+"git review src/test.go\nexit\n"
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(script)),&out);err!=nil{t.Fatal(err)}
 if len(f.actions)!=5||strings.Contains(strings.Join(f.actions,","),"diff:") {
  t.Fatalf("git review used last quota action for partial review: %v",f.actions)
 }
 if !strings.Contains(out.String(),"cần hai lượt E2EE"){
  t.Fatalf("missing quota refusal: %q",out.String())
 }
}

func TestInteractiveGitReviewRejectsTraversalAndGitFlags(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 script:="git review ../../secret\ngit review /etc/passwd\ngit review --no-pager\ngit review file\u202e.txt\ngit review a\x1b[31m\nexit\n"
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(script)),&out);err!=nil{t.Fatal(err)}
 if len(f.actions)!=0{t.Fatalf("unsafe Git flags or path reached host: %v",f.actions)}
}

func TestInteractiveGitReviewEscapesBothRemoteDiffContents(t *testing.T) {
 f:=&fakeInteractiveTaskDeck{unstagedDiff:"\x1b[31mred\n",stagedDiff:"\u202ebidi"}
 var out bytes.Buffer
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader("git review source.txt\nexit\n")),&out);err!=nil{t.Fatal(err)}
 got:=out.String()
 if strings.Contains(got,"\x1b")||strings.Contains(got,"\u202e")||
  !strings.Contains(got,"\\u001b")||!strings.Contains(got,"\\u202e"){
  t.Fatalf("Git output spoofed native console: %q",got)
 }
}

func TestInteractiveGitReviewStopsAfterFirstRevokedAction(t *testing.T){
 f:=&fakeInteractiveTaskDeck{denied:true}
 var out bytes.Buffer
 err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader("git review source.txt\nexit\n")),&out)
 if err==nil || len(f.actions)!=1 || f.actions[0]!="git:diff:source.txt" {
  t.Fatalf("revoked native session accessed staged diff: err=%v actions=%v",err,f.actions)
 }
 if strings.Contains(out.String(),"-- Chưa stage")||strings.Contains(out.String(),"-- Đã stage"){
  t.Fatal("partial review was printed despite authorization failure")
 }
}

func TestInteractiveGitReviewStopsOnSecondRevokedActionWithoutPartialOutput(t *testing.T){
 f:=&fakeInteractiveTaskDeck{unstagedDiff:"confidential diff",denyStaged:true}
 var out bytes.Buffer
 err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader("git review source.txt\nexit\n")),&out)
 if err==nil||len(f.actions)!=2{
  t.Fatalf("second authorization refusal did not stop: err=%v actions=%v",err,f.actions)
 }
 if strings.Contains(out.String(),"confidential diff")||strings.Contains(out.String(),"-- Chưa stage")||
  strings.Contains(out.String(),"-- Đã stage"){
  t.Fatalf("partial review displayed when second read was revoked: %q",out.String())
 }
}

func TestInteractiveGitWriteAndShellSyntaxRemainUnavailable(t *testing.T) {
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 commands:=[]string{
  "git add src/config.go",
  "git restore --staged src/config.go",
  "git reset --hard HEAD",
  "git checkout -- src/config.go",
  "git commit -m approved",
  "git push origin main",
  "git fetch origin",
  "git pull --ff-only",
  "terminal git status",
  "terminal task 1; git add .",
 }
 script:=strings.Join(commands,"\n")+"\nexit\n"
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(script)),&out);err!=nil{t.Fatal(err)}
 if len(f.actions)!=0{
  t.Fatalf("unreviewed Git write/shell command was forwarded to host: %+v",f.actions)
 }
 if !strings.Contains(out.String(),"Lệnh không hỗ trợ"){
  t.Fatalf("unsupported Git writes not explicitly refused: %q",out.String())
 }
}

func TestInteractiveGitHeadDiffUsesSingleTypedActionAndSafePath(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader("cd folder with spaces\ngit diff --head name with spaces.go\nexit\n")),&out);err!=nil{
  t.Fatal(err)
 }
 if got:=strings.Join(f.actions,",");got!="git:head:folder with spaces/name with spaces.go"{
  t.Fatalf("incorrect full HEAD diff dispatch: %q",got)
 }
 for _,script:=range []string{
  "git diff --head ../../secret\nexit\n",
  "git diff --head /etc/passwd\nexit\n",
  "git diff --head --cached\nexit\n",
  "git diff --head file\u202e.go\nexit\n",
 }{
  g:=&fakeInteractiveTaskDeck{}
  var printed bytes.Buffer
  if err:=runNativeInteractiveConsole(context.Background(),g,
   bufio.NewReader(strings.NewReader(script)),&printed);err!=nil{t.Fatal(err)}
  if len(g.actions)!=0{t.Fatalf("invalid Git HEAD diff went to host: %q %v",script,g.actions)}
 }
}

func TestInteractiveGitStageRequiresPreviewAndExactConfirmation(t *testing.T){
 f:=&fakeInteractiveTaskDeck{previewText:"one\r\ntwo\n"}
 var out bytes.Buffer
 script:="cd folder with spaces\ngit stage source name.txt\nSTAGE folder with spaces/source name.txt\nexit\n"
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(script)),&out);err!=nil{t.Fatal(err)}
 if got:=strings.Join(f.actions,",");got!="preview:folder with spaces/source name.txt,git:stage:folder with spaces/source name.txt"{
  t.Fatalf("stage was not limited to preview and typed E2EE write: %s",got)
 }
 sum:=sha256.Sum256([]byte(f.previewText))
 if f.replaceSHA!=hex.EncodeToString(sum[:]){t.Fatalf("Git stage did not pass preview CAS hash: %q",f.replaceSHA)}
 if !strings.Contains(out.String(),"STAGE folder with spaces/source name.txt"){
  t.Fatalf("Git stage did not require path-scoped confirmation: %q",out.String())
 }
}

func TestInteractiveGitStageRejectsCancelTraversalAndQuota(t *testing.T){
 for _,script:=range []string{
  "git stage source.txt\nNO\nexit\n",
  "git stage source.txt\nSTAGE wrong.txt\nexit\n",
  "git stage ../../secret\nexit\n",
  "git stage /etc/passwd\nexit\n",
  "git stage --help\nexit\n",
  strings.Repeat("git status\n",5)+"git stage source.txt\nexit\n",
 }{
  f:=&fakeInteractiveTaskDeck{previewText:"safe\n"}
  var out bytes.Buffer
  if err:=runNativeInteractiveConsole(context.Background(),f,
   bufio.NewReader(strings.NewReader(script)),&out);err!=nil{t.Fatal(err)}
  if strings.Contains(strings.Join(f.actions,","),"git:stage:"){
   t.Fatalf("unsafe/cancelled/overquota stage went to host: %v",f.actions)
  }
  if strings.Contains(script,"git status")&&len(f.actions)!=5 {
   t.Fatalf("stage consumed first half of quota then refused: %v",f.actions)
  }
 }
}

func TestInteractiveGitStageEscapesMaliciousPreview(t *testing.T){
 f:=&fakeInteractiveTaskDeck{previewText:"\x1b[31mred\n"}
 var out bytes.Buffer
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader("git stage source.txt\nno\nexit\n")),&out);err!=nil{
  t.Fatal(err)
 }
 if strings.Contains(out.String(),"\x1b")||!strings.Contains(out.String(),"\\u001b"){
  t.Fatalf("malicious source preview controlled native terminal: %q",out.String())
 }
 if len(f.actions)!=1||f.actions[0]!="preview:source.txt"{
  t.Fatalf("cancelled stage reached write: %v",f.actions)
 }
}

func TestInteractiveGitUnstageRequiresStagedReviewConfirmationAndCAS(t *testing.T){
 f:=&fakeInteractiveTaskDeck{stagedDiff:"diff --git a/f b/f\n-old\n+new\n"}
 var out bytes.Buffer
 script:="cd group with spaces\ngit unstage item name.txt\nUNSTAGE group with spaces/item name.txt\nexit\n"
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(script)),&out);err!=nil{t.Fatal(err)}
 want:="git:staged:group with spaces/item name.txt,git:unstage:group with spaces/item name.txt"
 if got:=strings.Join(f.actions,",");got!=want{t.Fatalf("wrong typed E2EE actions: %s",got)}
 digest:=sha256.Sum256([]byte(f.stagedDiff))
 if f.replaceSHA!=hex.EncodeToString(digest[:]){t.Fatalf("incorrect staged diff CAS: %q",f.replaceSHA)}
 if !strings.Contains(out.String(),"UNSTAGE group with spaces/item name.txt")||!strings.Contains(out.String(),"-old"){
  t.Fatalf("missing path confirmation or staged preview: %s",out.String())
 }
}

func TestInteractiveGitUnstageCancelEmptyAndQuotaNeverWrite(t *testing.T){
 for _,script:=range []string{
  "git unstage file.txt\nno\nexit\n",
  "git unstage file.txt\nUNSTAGE wrong.txt\nexit\n",
  "git unstage ../../private\nexit\n",
  "git unstage /etc/passwd\nexit\n",
  "git unstage --help\nexit\n",
  strings.Repeat("git status\n",5)+"git unstage file.txt\nexit\n",
 }{
  f:=&fakeInteractiveTaskDeck{stagedDiff:"diff --git\n"}
  var out bytes.Buffer
  if err:=runNativeInteractiveConsole(context.Background(),f,
   bufio.NewReader(strings.NewReader(script)),&out);err!=nil{t.Fatal(err)}
  if strings.Contains(strings.Join(f.actions,","),"git:unstage:"){
   t.Fatalf("invalid/overquota/cancelled unstage reached index write: %v",f.actions)
  }
  if strings.Contains(script,"git status")&&len(f.actions)!=5{
   t.Fatalf("overquota unstage partially read index: %v",f.actions)
  }
 }
 f:=&fakeInteractiveTaskDeck{emptyStaged:true}
 var out bytes.Buffer
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader("git unstage file.txt\nexit\n")),&out);err!=nil{t.Fatal(err)}
 if got:=strings.Join(f.actions,",");got!="git:staged:file.txt"{
  t.Fatalf("empty diff must not request index mutation: %q",got)
 }
 if !strings.Contains(out.String(),"Không có thay đổi đã stage"){
  t.Fatalf("empty staged diff not explained: %q",out.String())
 }
}

func TestInteractiveGitUnstageEscapesUntrustedStagedDiff(t *testing.T){
 f:=&fakeInteractiveTaskDeck{stagedDiff:"\x1b[31mchange\n"}
 var out bytes.Buffer
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader("git unstage file.txt\nCANCEL\nexit\n")),&out);err!=nil{t.Fatal(err)}
 if strings.Contains(out.String(),"\x1b")||!strings.Contains(out.String(),"\\u001b"){
  t.Fatalf("Git diff can spoof console: %q",out.String())
 }
 if len(f.actions)!=1||f.actions[0]!="git:staged:file.txt"{t.Fatalf("cancel reached write: %v",f.actions)}
}

func TestInteractiveGitCommitReviewUsesOneReadOnlyTypedE2EEAction(t *testing.T){
 fake:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 if err:=runNativeInteractiveConsole(context.Background(),fake,
  bufio.NewReader(strings.NewReader("git commit-review\nexit\n")),&out);err!=nil{
  t.Fatal(err)
 }
 if got:=strings.Join(fake.actions,",");got!="git:commit-review"{
  t.Fatalf("unexpected host Git write or multiple actions: %v",fake.actions)
 }
}
func TestInteractiveGitCommitReviewRejectsCommitShellSyntax(t *testing.T){
 fake:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 for _,cmd:=range []string{"git commit -m injected","git commit --amend",
  "git commit-review -m unsafe","git commit-review && git push",
  "git commit-review; rm -rf .","git commit-tree HEAD","git commit -F /tmp/secret"}{
  if err:=runNativeInteractiveConsole(context.Background(),fake,
   bufio.NewReader(strings.NewReader(cmd+"\nexit\n")),&out);err!=nil{
   t.Fatal(err)
  }
 }
 if len(fake.actions)!=0{
  t.Fatalf("unreviewed Git command was forwarded to host: %v",fake.actions)
 }
}

func TestInteractiveGitCommitMustReviewConfirmHeadAndSHA(t *testing.T){
 const head="0123456789abcdef0123456789abcdef01234567"
 review:="HEAD "+head+"\nBRANCH refs/heads/main\nINDEX_SHA256 "+
  strings.Repeat("a",64)+"\n-- COMPLETE STAGED DIFF --\ndiff --git a/a.txt b/a.txt\n+hi\n"
 f:=&fakeInteractiveTaskDeck{commitReview:review}
 var output bytes.Buffer
 script:="git commit\nSafe commit message\nCOMMIT 0123456789ab\nexit\n"
 err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(script)),&output)
 if err!=nil{t.Fatal(err)}
 if got:=strings.Join(f.actions,",");got!="git:commit-review,git:commit"{
  t.Fatalf("Git commit didn't use two distinct E2EE actions: %q",got)
 }
 h:=sha256.Sum256([]byte(review))
 if f.replaceSHA!=hex.EncodeToString(h[:])||f.commitMessage!="Safe commit message"{
  t.Fatalf("commit used wrong review CAS or message: %s %q",f.replaceSHA,f.commitMessage)
 }
 if !strings.Contains(output.String(),"COMMIT 0123456789ab")||
  !strings.Contains(output.String(),"+hi"){t.Fatal("missing full review or HEAD confirmation")}
}

func TestInteractiveGitCommitRejectsCancellationInvalidMessageAndQuota(t *testing.T){
 const head="0123456789abcdef0123456789abcdef01234567"
 review:="HEAD "+head+"\nBRANCH refs/heads/main\nINDEX_SHA256 "+
  strings.Repeat("a",64)+"\n-- COMPLETE STAGED DIFF --\n+hi\n"
 for _,input:=range []string{
  "git commit\nGood message\nCANCEL\nexit\n",
  "git commit\nGood message\nCOMMIT badhead\nexit\n",
  "git commit\n$()\nexit\n",
  "git commit\nBad\nCOMMIT 000000000000\nexit\n",
  strings.Repeat("git status\n",5)+"git commit\nexit\n",
 }{
  f:=&fakeInteractiveTaskDeck{commitReview:review}
  var out bytes.Buffer
  if err:=runNativeInteractiveConsole(context.Background(),f,
   bufio.NewReader(strings.NewReader(input)),&out);err!=nil{t.Fatal(err)}
  if strings.Contains(strings.Join(f.actions,","),"git:commit,")||
   (len(f.actions)>0&&f.actions[len(f.actions)-1]=="git:commit"){
   t.Fatalf("unauthorized commit went to host: %+v",f.actions)
  }
  if strings.Contains(input,"git status")&&len(f.actions)!=5{
   t.Fatalf("overquota commit consumed partial authorization: %v",f.actions)
  }
 }
}

func TestInteractiveGitCommitEscapesReviewControlCharacters(t *testing.T){
 const head="0123456789abcdef0123456789abcdef01234567"
 f:=&fakeInteractiveTaskDeck{commitReview:"HEAD "+head+
  "\nBRANCH refs/heads/main\nINDEX_SHA256 "+strings.Repeat("a",64)+
  "\n-- COMPLETE STAGED DIFF --\n\x1b[31mspoof\n"}
 var out bytes.Buffer
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader("git commit\nValid message\nNO\nexit\n")),&out);err!=nil{t.Fatal(err)}
 if strings.Contains(out.String(),"\x1b")||!strings.Contains(out.String(),"\\u001b"){
  t.Fatalf("untrusted staged diff spoofed native console: %q",out.String())
 }
}

func TestNativeBranchDeleteNeedsReviewAndExactConfirmation(t *testing.T){
 for _,kind:=range []string{"accept","cancel","stale","wrong","already-current","quota"}{
  t.Run(kind,func(t *testing.T){
   f:=&fakeInteractiveTaskDeck{}
   oid:=strings.Repeat("a",40)
   review:="HEAD "+oid+"\nBRANCH refs/heads/main\nTARGET refs/heads/taskdeck/safe-target"+
    "\nTARGET_HEAD "+oid+"\nINDEX_SHA256 "+strings.Repeat("b",64)+"\nCLEAN yes\n"
   script:="git branch-delete taskdeck/safe-target\n"
   switch kind{
   case "accept":script+="DELETE BRANCH taskdeck/safe-target "+oid[:12]+"\n"
   case "cancel":script+="NO\n"
   case "stale":f.deleteReview=strings.Replace(review,"TARGET_HEAD "+oid,"TARGET_HEAD "+strings.Repeat("c",40),1)
   case "wrong":script+="DELETE BRANCH taskdeck/other "+oid[:12]+"\n"
   case "already-current":f.deleteReview=strings.Replace(review,"BRANCH refs/heads/main","BRANCH refs/heads/taskdeck/safe-target",1)
   case "quota":script=strings.Repeat("git status\n",5)+script
   }
   script+="exit\n"
   var out bytes.Buffer
   err:=runNativeInteractiveConsole(context.Background(),f,bufio.NewReader(strings.NewReader(script)),&out)
   if kind=="stale"||kind=="already-current"{
    if err==nil{t.Fatal("unsafe host review accepted")}
   }else if err!=nil{t.Fatal(err)}
   seenWrite:=false
   for _,a:=range f.actions{if a=="git:branch-delete"{seenWrite=true}}
   if seenWrite!=(kind=="accept"){t.Fatalf("unexpected delete %s: %+v",kind,f.actions)}
   if kind=="accept"{
    sum:=sha256.Sum256([]byte(review))
    if f.deleteSlug!="safe-target"||f.replaceSHA!=hex.EncodeToString(sum[:]){t.Fatal("untrusted review SHA")}
   }
  })
 }
}

func TestNativeBranchDeleteRejectsUntrustedSlugsBeforeE2EE(t *testing.T){
 for _,script:=range []string{
  "git branch-delete taskdeck/../main\n",
  "git branch-delete taskdeck/-force\n",
  "git branch-delete taskdeck/a/b\n",
  "git branch-delete main\n",
 }{
  f:=&fakeInteractiveTaskDeck{}
  var out bytes.Buffer
  if err:=runNativeInteractiveConsole(context.Background(),f,
   bufio.NewReader(strings.NewReader(script+"exit\n")),&out);err!=nil{t.Fatal(err)}
  if len(f.actions)!=0{t.Fatalf("unauthorized branch action: %+v",f.actions)}
 }
}

func TestNativeLargeLineEditReadsHostPageThenRequiresExactConfirmation(t *testing.T){
 sha:=strings.Repeat("a",64)
 for _,tc:=range []struct{name,confirmation string;expectWrite bool}{
  {"accepted","EDIT LINE 3 "+sha[:12],true},
  {"canceled","NO",false},
  {"wrong-line","EDIT LINE 4 "+sha[:12],false},
 }{
  t.Run(tc.name,func(t *testing.T){
   f:=&fakeInteractiveTaskDeck{}
   f.linePageVersions=[]string{sha}
   f.linePageTotal=3
   var out bytes.Buffer
   input:="edit-large-line source.txt 3\nnew content\n"+tc.confirmation+"\nexit\n"
   err:=runNativeInteractiveConsole(context.Background(),f,
    bufio.NewReader(strings.NewReader(input)),&out)
   if err!=nil{t.Fatal(err)}
   writes:=0
   for _,a:=range f.actions{if strings.HasPrefix(a,"edit-large-line:"){writes++}}
   if (writes==1)!=tc.expectWrite{t.Fatalf("wrong dispatch: %+v",f.actions)}
   if tc.expectWrite{
    if f.replaceText!="new content"||f.replaceSHA!=sha||f.replacePath!="source.txt"{
     t.Fatalf("wrong E2EE CAS request: %+v",f)
    }
   }
  })
 }
}
func TestNativeLargeLineEditBadInputAndInsufficientQuotaNeverWrites(t *testing.T){
 for _,command:=range []string{
  "edit-large-line ../escape 2\n",
  "edit-large-line source.txt 0\n",
  "edit-large-line source.txt -1\n",
  "edit-large-line source.txt +2\n",
  "edit-large-line source.txt 1;id\n",
 }{
  f:=&fakeInteractiveTaskDeck{}
  var out bytes.Buffer
  err:=runNativeInteractiveConsole(context.Background(),f,
   bufio.NewReader(strings.NewReader(command+"exit\n")),&out)
  if err!=nil{t.Fatal(err)}
  if len(f.actions)!=0{t.Fatalf("unsafe input issued action: %+v",f.actions)}
 }
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 script:=strings.Repeat("git status\n",5)+"edit-large-line source.txt 2\nexit\n"
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(script)),&out);err!=nil{t.Fatal(err)}
 if len(f.actions)!=5{t.Fatalf("insufficient E2EE quota sent mutation: %+v",f.actions)}
}

func TestNativeTerminalTaskReportRequiresCatalogueAndExactConfirmation(t *testing.T){
 for _,tc:=range []struct{name,input string;want string}{
  {"accepted","terminal task-report 1\nRUN TASK 1\nexit\n","task:list,task-report:1"},
  {"canceled","terminal task-report 1\nNO\nexit\n","task:list"},
  {"no-leading-zero","terminal task-report 01\nexit\n",""},
  {"reject-arbitrary-command","terminal task-report id;sh\nexit\n",""},
 }{
  t.Run(tc.name,func(t *testing.T){
   f:=&fakeInteractiveTaskDeck{}
   var out bytes.Buffer
   err:=runNativeInteractiveConsole(context.Background(),f,
    bufio.NewReader(strings.NewReader(tc.input)),&out)
   if err!=nil{t.Fatal(err)}
   if got:=strings.Join(f.actions,",");got!=tc.want{
    t.Fatalf("remote command bypassed host review: %q != %q",got,tc.want)
   }
  })
 }
}

func TestNativeManagedTaskOnlyFixedHostTasksAndSessionScopedCancel(t *testing.T){
 handle:=strings.Repeat("a",32)
 for _,tc:=range []struct{name,input,want string}{
  {"start","terminal task-start 1\nSTART TASK 1\nexit\n","task:list,managed:start:1"},
  {"start-canceled","terminal task-start 1\nNO\nexit\n","task:list"},
  {"start-injection","terminal task-start 1;sh\nexit\n",""},
  {"status","terminal task-status "+handle+"\nexit\n","managed:status:"+handle},
  {"cancel-confirmed","terminal task-cancel "+handle+"\nCANCEL TASK "+handle[:12]+"\nexit\n","managed:cancel:"+handle},
  {"cancel-rejected","terminal task-cancel "+handle+"\nWRONG\nexit\n",""},
  {"pid-denied","terminal task-cancel 1111\nexit\n",""},
 }{
  t.Run(tc.name,func(t *testing.T){
   f:=&fakeInteractiveTaskDeck{}
   var out bytes.Buffer
   err:=runNativeInteractiveConsole(context.Background(),f,
    bufio.NewReader(strings.NewReader(tc.input)),&out)
   if err!=nil{t.Fatal(err)}
   if got:=strings.Join(f.actions,",");got!=tc.want{
    t.Fatalf("unsafe managed task console routing: got %q want %q",got,tc.want)
   }
  })
 }
}

func TestNativeManagedOutputRequiresExactHandleAndByteOffset(t *testing.T){
 handle:=strings.Repeat("b",32)
 for _,tc:=range []struct{name,input,want string}{
  {"read-from-start","terminal task-output "+handle+" 0\nexit\n","managed:output:"+handle+":0"},
  {"read-next","terminal task-output "+handle+" 2048\nexit\n","managed:output:"+handle+":2048"},
  {"bad-handle","terminal task-output 1234 0\nexit\n",""},
  {"negative","terminal task-output "+handle+" -1\nexit\n",""},
  {"leading-zero","terminal task-output "+handle+" 0004\nexit\n",""},
  {"arbitrary-shell","terminal task-output "+handle+" 5;id\nexit\n",""},
  {"overflow","terminal task-output "+handle+" 1073741825\nexit\n",""},
 }{
  t.Run(tc.name,func(t *testing.T){
   f:=&fakeInteractiveTaskDeck{}
   var out bytes.Buffer
   if err:=runNativeInteractiveConsole(context.Background(),f,
    bufio.NewReader(strings.NewReader(tc.input)),&out);err!=nil{t.Fatal(err)}
   if got:=strings.Join(f.actions,",");got!=tc.want{
    t.Fatalf("unsafe terminal cursor accepted: got %q want %q",got,tc.want)
   }
  })
 }
}

func TestNativeLargeLineDeleteRequiresExactConfirmation(t *testing.T){
 for _,tc:=range []struct{name,confirm string;writes int}{
  {"confirm","DELETE LINE 4 "+strings.Repeat("a",12),1},
  {"wrong-sha","DELETE LINE 4 "+strings.Repeat("b",12),0},
  {"wrong-line","DELETE LINE 5 "+strings.Repeat("a",12),0},
  {"cancel","NO",0},
 }{
  t.Run(tc.name,func(t *testing.T){
   fake:=&fakeInteractiveTaskDeck{}
   input:="delete-large-line source.txt 4\\n"+tc.confirm+"\\nexit\\n"
   // Literal newline delimiters are part of the console protocol.
   input=strings.ReplaceAll(input,"\\n","\n")
   var out bytes.Buffer
   err:=runNativeInteractiveConsole(context.Background(),fake,
    bufio.NewReader(strings.NewReader(input)),&out)
   if err!=nil{t.Fatal(err)}
   count:=0
   for _,act:=range fake.actions{
    if strings.HasPrefix(act,"delete-large-line:"){count++}
   }
   if count!=tc.writes{t.Fatalf("wrong write count %d: %v, output: %s",count,fake.actions,out.String())}
   if len(fake.actions)==0||fake.actions[0]!="view-lines:source.txt:4"{
    t.Fatalf("missing authenticated page: %v",fake.actions)
   }
   if count==1 && fake.replaceSHA!=strings.Repeat("a",64){
    t.Fatal("client failed to use full source SHA")
   }
  })
 }
}

func TestNativeLargeLineDeleteDeniesQuotaAndPath(t *testing.T){
 fake:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 err:=runNativeInteractiveConsole(context.Background(),fake,bufio.NewReader(strings.NewReader(
  "delete-large-line ../secret 1\ndelete-large-line source.txt 0\nexit\n",
 )),&out)
 if err!=nil{t.Fatal(err)}
 if len(fake.actions)!=0{t.Fatalf("unsafe path/line reached E2EE: %v",fake.actions)}
}

func TestNativeLargeLineInsertRequiresExactReview(t *testing.T){
 for _,tc:=range []struct{name,confirm string;writes int}{
  {"yes","INSERT LINE 4 "+strings.Repeat("a",12),1},
  {"bad-sha","INSERT LINE 4 "+strings.Repeat("b",12),0},
  {"bad-index","INSERT LINE 5 "+strings.Repeat("a",12),0},
  {"reject","NO",0},
 }{
  t.Run(tc.name,func(t *testing.T){
   f:=&fakeInteractiveTaskDeck{}
   input:="insert-large-line source.txt 4\nnew source\n"+tc.confirm+"\nexit\n"
   var out bytes.Buffer
   if err:=runNativeInteractiveConsole(context.Background(),f,
    bufio.NewReader(strings.NewReader(input)),&out);err!=nil{t.Fatal(err)}
   count:=0
   for _,a:=range f.actions{if strings.HasPrefix(a,"insert-large-line:"){count++}}
   if count!=tc.writes{t.Fatalf("unexpected insert count %d, actions %v, output %s",count,f.actions,out.String())}
   if len(f.actions)==0||f.actions[0]!="view-lines:source.txt:4"{t.Fatal("missing SHA page review")}
   if count==1&&(f.replaceSHA!=strings.Repeat("a",64)||f.replaceText!="new source"){
    t.Fatal("host insert request did not contain reviewed source SHA and literal line")
   }
  })
 }
}
func TestNativeLargeLineInsertRejectsInvalidSourceArguments(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 err:=runNativeInteractiveConsole(context.Background(),f,bufio.NewReader(strings.NewReader(
  "insert-large-line ../outside.txt 1\ninsert-large-line source.txt 0\ninsert-large-line source.txt 0002\nexit\n",
  )),&out)
 if err!=nil{t.Fatal(err)}
 if len(f.actions)!=0{t.Fatalf("unsafe large line insert reached agent: %v",f.actions)}
}
