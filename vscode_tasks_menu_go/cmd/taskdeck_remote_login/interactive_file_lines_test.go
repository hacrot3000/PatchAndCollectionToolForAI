package main

import (
 "bufio"
 "bytes"
 "context"
 "strings"
 "testing"
)

func TestInteractiveFileLinesParsesStrictPages(t *testing.T){
 hash:=strings.Repeat("a",64)
 good:="FILE_SHA256 "+hash+
  "\nSTART_LINE 7\nNEXT_LINE 9\nTOTAL_LINES 12"+
  "\n-- COMPLETE SOURCE LINES --\n7: 你好\n8: hello\n"
 if got,err:=consoleLinePageSHA(good,7);err!=nil||got!=hash{
  t.Fatalf("complete page rejected: %q %v",got,err)
 }
 for _,bad:=range []string{
  strings.Replace(good,"START_LINE 7","START_LINE 8",1),
  strings.Replace(good,"NEXT_LINE 9","NEXT_LINE 10",1),
  strings.Replace(good,"TOTAL_LINES 12","TOTAL_LINES 8",1),
  strings.Replace(good,"FILE_SHA256 "+hash,"FILE_SHA256 "+strings.Repeat("A",64),1),
  strings.Replace(good,"7: 你好","9: 你好",1),
  strings.Replace(good,"-- COMPLETE SOURCE LINES --","-- PARTIAL --",1),
  strings.TrimSuffix(good,"\n"),
  strings.Repeat("x",4097),
 }{
  if _,err:=consoleLinePageSHA(bad,7);err==nil{
   t.Fatalf("malformed/partial source page accepted: %q",bad)
  }
 }
 eof:="FILE_SHA256 "+hash+"\nSTART_LINE 12\nNEXT_LINE 0\nTOTAL_LINES 12\n-- COMPLETE SOURCE LINES --\n12: end\n"
 if _,err:=consoleLinePageSHA(eof,12);err!=nil{t.Fatal(err)}
}

func TestInteractiveFileLinesPaginatesWithoutDownloadOrShell(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 input:="view-lines source file with spaces.go 7\nview-lines source file with spaces.go 9\nexit\n"
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(input)),&out);err!=nil{t.Fatal(err)}
 if got:=strings.Join(f.actions,",");got!="view-lines:source file with spaces.go:7,view-lines:source file with spaces.go:9"{
  t.Fatalf("native paged reader forwarded unexpected operation: %s",got)
 }
 if !strings.Contains(out.String(),"FILE_SHA256 ")||
  !strings.Contains(out.String(),"9: example"){
  t.Fatalf("source lines not displayed: %q",out.String())
 }
}

func TestInteractiveFileLinesDetectsStaleSourceSHA(t *testing.T){
 f:=&fakeInteractiveTaskDeck{linePageVersions:[]string{
  strings.Repeat("a",64),strings.Repeat("b",64),
 }}
 var out bytes.Buffer
 input:="view-lines a.go 1\nview-lines a.go 2\nexit\n"
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(input)),&out);err!=nil{t.Fatal(err)}
 if len(f.actions)!=2||!strings.Contains(out.String(),"đã đổi SHA")||
  strings.Contains(out.String(),"2: example"){
  t.Fatalf("client silently mixed file revisions: actions=%v output=%q",f.actions,out.String())
 }
}

func TestInteractiveFileLinesRejectsShellAndInvalidStartBeforeNetwork(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 for _,command:=range []string{
  "view-lines a.go 0","view-lines a.go -1","view-lines a.go 01",
  "view-lines a.go 2097153","view-lines ../secret 1",
  "view-lines /etc/passwd 1","view-lines a.go 1; cat /etc/passwd",
  "view-lines a.go","view-lines a.go 1 --exec=sh",
 }{
  if err:=runNativeInteractiveConsole(context.Background(),f,
   bufio.NewReader(strings.NewReader(command+"\nexit\n")),&out);err!=nil{t.Fatal(err)}
 }
 if len(f.actions)!=0{
  t.Fatalf("invalid file page command was sent to host: %v",f.actions)
 }
}

func TestInteractiveFileLinesEscapesTerminalControls(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader("view-lines safe.go 1\nexit\n")),&out);err!=nil{t.Fatal(err)}
 if len(f.actions)!=1||!strings.Contains(out.String(),"-- COMPLETE SOURCE LINES --"){
  t.Fatalf("missing authenticated page: %s",out.String())
 }
}
