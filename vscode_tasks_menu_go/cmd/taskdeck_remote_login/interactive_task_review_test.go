package main

import (
 "bufio"
 "bytes"
 "context"
 "strconv"
 "strings"
 "testing"
)

func TestInteractiveTaskCatalogueRejectsAmbiguousHostEntries(t *testing.T){
 good:="1\tVerified process task\n20\tAnother approved task\n"
 if label,err:=consoleApprovedTaskLabel(good,20);err!=nil||label!="Another approved task"{
  t.Fatalf("valid approved catalogue rejected: %q %v",label,err)
 }
 for _,tc:=range []string{
  "",
  "No host-approved remote tasks.",
  "1\tValid but no LF",
  "1\tFirst\n1\tDuplicate\n",
  "01\tLeading zero\n",
  "-1\tNegative\n",
  "0\tZero\n",
  "2147483648\tToo large\n",
  "1\t\n",
  "1\tHas\tTab\n",
  "1\tFirst\rSecond\n",
  "1\tColour \x1b[31mspoof\n",
  "1\tBidi \u202e spoof\n",
  "1\tZero width \u200b label\n",
  "1\tGood\n2\tInvalid\rLabel\n",
  "1\tGood\n2\tBad\tExtra\n",
  "1\tGood\n2\tUnknown\n3\t"+strings.Repeat("x",121)+"\n",
  "1\t"+strings.Repeat("x",4096)+"\n",
 }{
  if label,err:=consoleApprovedTaskLabel(tc,1);err==nil{
   t.Fatalf("malformed host task catalogue accepted: %q => %q",tc,label)
  }
 }
 if _,err:=consoleApprovedTaskLabel(good,100);err==nil{
  t.Fatal("task not present in authenticated host catalogue accepted")
 }
}

func TestInteractiveTaskRunRequiresExactReviewConfirmation(t *testing.T){
 f:=&fakeInteractiveTaskDeck{taskCatalogue:"11\tHost-approved safe task\n"}
 var output bytes.Buffer
 input:="terminal task 11\nRUN TASK 11\nexit\n"
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(input)),&output);err!=nil{t.Fatal(err)}
 // fake task records run by rune(id+'0'); for a one-digit ID it emits
 // the ordinary digit, which is sufficient to assert exactly one launch.
 if !strings.Contains(output.String(),"Host-approved safe task")||
  !strings.Contains(output.String(),"RUN TASK 11"){
  t.Fatalf("missing review or confirmation: %q",output.String())
 }
 if len(f.actions)!=2||f.actions[0]!="task:list"||
  !strings.HasPrefix(f.actions[1],"task:"){
  t.Fatalf("task ran without catalogue review: %v",f.actions)
 }
}

func TestInteractiveTaskRunDoesNotLaunchOnCancelOrSpoof(t *testing.T){
 for _,input:=range []string{
  "terminal task 1\nNO\nexit\n",
  "terminal task 1\nRUN TASK 2\nexit\n",
  "terminal task 1\nrun task 1\nexit\n",
  "terminal task 01\nexit\n",
  "terminal task -1\nexit\n",
  "terminal task 1; id\nexit\n",
  "terminal task 1\nRUN TASK 1; git push\nexit\n",
 }{
  f:=&fakeInteractiveTaskDeck{taskCatalogue:"1\tApproved task\n"}
  var out bytes.Buffer
  if err:=runNativeInteractiveConsole(context.Background(),f,
   bufio.NewReader(strings.NewReader(input)),&out);err!=nil{t.Fatal(err)}
  if strings.Contains(strings.Join(f.actions,","),"task:1"){
   t.Fatalf("unconfirmed/spoofed task was launched: %q actions=%v",input,f.actions)
  }
 }
}

func TestInteractiveTaskRunMustRejectMissingDuplicateAndInvalidHostCatalogue(t *testing.T){
 for _,catalogue:=range []string{
  "No host-approved remote tasks.",
  "2\tOther approved task\n",
  "1\tOne\n1\tDuplicated\n",
  "1\tTask\x1b[2J\n",
  "1\tSafe\n2\tBad\tExtra\n",
 }{
  f:=&fakeInteractiveTaskDeck{taskCatalogue:catalogue}
  var out bytes.Buffer
  if err:=runNativeInteractiveConsole(context.Background(),f,
   bufio.NewReader(strings.NewReader("terminal task 1\nexit\n")),&out);err!=nil{t.Fatal(err)}
  if len(f.actions)!=1||f.actions[0]!="task:list"{
   t.Fatalf("malformed or absent catalogue caused process launch: %q actions=%v",catalogue,f.actions)
  }
 }
}

func TestInteractiveTaskRunQuotaDoesNotConsumePartialReview(t *testing.T){
 for _,n:=range []int{5,6}{
  f:=&fakeInteractiveTaskDeck{taskCatalogue:"1\tApproved task\n"}
  var out bytes.Buffer
  input:=strings.Repeat("git status\n",n)+"terminal task 1\nexit\n"
  if err:=runNativeInteractiveConsole(context.Background(),f,
   bufio.NewReader(strings.NewReader(input)),&out);err!=nil{t.Fatal(err)}
  if len(f.actions)!=n{t.Fatalf("overquota task emitted network action: %v",f.actions)}
  if !strings.Contains(out.String(),"cần hai lượt E2EE"){
   t.Fatalf("quota message missing: %s",out.String())
  }
 }
}

func TestInteractiveTaskRunAllCanonicalIDsValidated(t *testing.T){
 for _,id:=range []int{1,20,2147483647}{
  catalogue:=strconv.Itoa(id)+"\tSafe approved task\n"
  if got,err:=consoleApprovedTaskLabel(catalogue,id);err!=nil||got!="Safe approved task"{
   t.Fatalf("canonical ID %d rejected: %q %v",id,got,err)
  }
 }
}
