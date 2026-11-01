package main

import (
 "bufio"
 "bytes"
 "context"
 "strings"
 "testing"
)

func TestInteractiveViewNextFollowsAuthenticatedPageCursorAndStopsAtEOF(t *testing.T){
 f:=&fakeInteractiveTaskDeck{linePageTotal:3}
 var out bytes.Buffer
 input:="view-next\nview-lines src/file with spaces.go 1\n"+
  "view-next\nview-next\nview-next\nexit\n"
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(input)),&out);err!=nil{t.Fatal(err)}
 want:="view-lines:src/file with spaces.go:1,"+
  "view-lines:src/file with spaces.go:2,"+
  "view-lines:src/file with spaces.go:3"
 if got:=strings.Join(f.actions,",");got!=want{
  t.Fatalf("view-next sent wrong host actions: %s",got)
 }
 if !strings.Contains(out.String(),"Không có trang tiếp theo")||
  !strings.Contains(out.String(),"3: example"){
  t.Fatalf("missing EOF indication or complete line: %q",out.String())
 }
}
func TestInteractiveViewNextPinsOriginalPathAcrossLocalCD(t *testing.T){
 f:=&fakeInteractiveTaskDeck{linePageTotal:2}
 var out bytes.Buffer
 input:="view-lines root.txt 1\ncd nested\nview-next\nexit\n"
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(input)),&out);err!=nil{t.Fatal(err)}
 if got:=strings.Join(f.actions,",");got!="view-lines:root.txt:1,view-lines:root.txt:2"{
  t.Fatalf("cursor used current local cwd instead of pinned source: %q",got)
 }
}
func TestInteractiveViewNextRejectsStaleSourceVersionBeforePrintingSecondPage(t *testing.T){
 f:=&fakeInteractiveTaskDeck{
  linePageTotal:3,linePageVersions:[]string{strings.Repeat("a",64),strings.Repeat("b",64)},
 }
 var out bytes.Buffer
 input:="view-lines a.go 1\nview-next\nview-next\nexit\n"
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(input)),&out);err!=nil{t.Fatal(err)}
 if len(f.actions)!=2||!strings.Contains(out.String(),"đã đổi SHA")||
  strings.Contains(out.String(),"2: example"){
  t.Fatalf("stale source printed/mixed pages: %+v %q",f.actions,out.String())
 }
}
func TestInteractiveViewNextCannotRequestUnreviewedPathOrShell(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 input:="view-next ../private\nview-next; sh\nview-next\nexit\n"
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(input)),&out);err!=nil{t.Fatal(err)}
 if len(f.actions)!=0{
  t.Fatalf("view-next accepted shell/path arguments: %+v",f.actions)
 }
}
func TestInteractiveViewNextQuotaRemainsOneE2EEActionPerPage(t *testing.T){
 f:=&fakeInteractiveTaskDeck{linePageTotal:100}
 var out bytes.Buffer
 input:="view-lines a.go 1\n"+strings.Repeat("view-next\n",8)+"exit\n"
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(input)),&out);err!=nil{t.Fatal(err)}
 if len(f.actions)!=6||!strings.Contains(out.String(),"Phiên đã dùng hết"){
  t.Fatalf("view-next bypassed E2EE quota: actions=%v output=%q",f.actions,out.String())
 }
}
