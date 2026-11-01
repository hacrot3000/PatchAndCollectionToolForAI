package main

import (
 "bufio"
 "bytes"
 "context"
 "crypto/sha256"
 "encoding/hex"
 "strings"
 "testing"
)

func TestInteractiveInsertLineRetainsOriginalLineEndingsAndAllBytes(t *testing.T){
 cases:=[]struct{
  original string
  line int
  content string
  want string
 }{
  {"alpha\r\nbeta\r\n",1,"head","head\r\nalpha\r\nbeta\r\n"},
  {"alpha\r\nbeta\r\n",2,"middle","alpha\r\nmiddle\r\nbeta\r\n"},
  {"alpha\r\nbeta\r\n",3,"tail","alpha\r\nbeta\r\ntail\r\n"},
  {"alpha\nbeta",3,"tail","alpha\nbeta\ntail"},
  {"alpha\nbeta",1,"head","head\nalpha\nbeta"},
  {"alpha",1,"head","head\nalpha"},
  {"alpha",2,"tail","alpha\ntail"},
  {"",1,"first","first\n"},
  {"",1,"","\n"},
  {"a\n\nc",2,"b","a\nb\n\nc"},
 }
 for _,tc:=range cases {
  got,err:=consoleInsertLine(tc.original,tc.line,tc.content)
  if err!=nil||got!=tc.want {
   t.Errorf("insert line %d in %q: got %q, error=%v; want %q",
    tc.line,tc.original,got,err,tc.want)
  }
 }
}

func TestInteractiveDeleteLinePreservesNeighboringBytesAndFinalNewline(t *testing.T){
 cases:=[]struct {
  original string
  line int
  removed,want string
 }{
  {"alpha\r\nbeta\r\n",1,"alpha","beta\r\n"},
  {"alpha\r\nbeta\r\n",2,"beta","alpha\r\n"},
  {"alpha\nbeta",1,"alpha","beta"},
  {"alpha\nbeta",2,"beta","alpha"},
  {"alpha\r\nbeta",2,"beta","alpha"},
  {"single",1,"single",""},
  {"only\n",1,"only",""},
  {"a\n\nc",2,"","a\nc"},
  {"a\n\nc",3,"c","a\n"},
 }
 for _,tc:=range cases {
  removed,got,err:=consoleDeleteLine(tc.original,tc.line)
  if err!=nil||removed!=tc.removed||got!=tc.want {
   t.Errorf("delete line %d from %q: removed=%q result=%q err=%v, want removed=%q result=%q",
    tc.line,tc.original,removed,got,err,tc.removed,tc.want)
  }
 }
}

func TestInteractiveLineInsertDeleteRefuseInvalidAndOversized(t *testing.T) {
 for _,tc:=range []struct{source string;line int;text string}{
  {"abc",0,"x"},
  {"abc",3,"x"},
  {"",2,"x"},
  {"abc",1,"\n"},
  {"abc",1,"\x1b[31m"},
  {"abc",1,"\u202eabc"},
  {strings.Repeat("x",4096),1,"a"},
  {"a",1,strings.Repeat("y",4096)},
 }{
  if result,err:=consoleInsertLine(tc.source,tc.line,tc.text);err==nil||result!=""{
   t.Errorf("invalid insertion was accepted %+v",tc)
  }
 }
 for _,tc:=range []struct{source string;line int}{
  {"",1},{"abc",0},{"abc",2},{"abc\n",2},
  {strings.Repeat("x",4097),1},
 }{
  if _,result,err:=consoleDeleteLine(tc.source,tc.line);err==nil||result!=""{
   t.Errorf("invalid deletion was accepted %+v",tc)
  }
 }
}

func TestInteractiveInsertLineUsesAuthenticatedPreviewFullCASAndExactConfirmation(t *testing.T){
 f:=&fakeInteractiveTaskDeck{previewText:"alpha\r\nbeta\r\n"}
 var out bytes.Buffer
 script:="insert-line folder/settings.txt\n2\nmiddle\nINSERT 2\nexit\n"
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(script)),&out);err!=nil{t.Fatal(err)}
 hash:=sha256.Sum256([]byte(f.previewText))
 if f.replacePath!="folder/settings.txt"||
  f.replaceText!="alpha\r\nmiddle\r\nbeta\r\n"||
  f.replaceSHA!=hex.EncodeToString(hash[:]) {
  t.Fatalf("insert failed full-file SHA-256 CAS: %+v",f)
 }
 if got:=strings.Join(f.actions,",");got!="preview:folder/settings.txt,edit:folder/settings.txt"{
  t.Fatalf("unexpected typed E2EE action sequence %q",got)
 }
 if !strings.Contains(out.String(),"Toàn bộ nội dung dự kiến") ||
  !strings.Contains(out.String(),"2: middle"){
  t.Fatalf("no authenticated preview before insert: %q",out.String())
 }
}

func TestInteractiveDeleteLineUsesAuthenticatedPreviewFullCASAndExactConfirmation(t *testing.T){
 f:=&fakeInteractiveTaskDeck{previewText:"alpha\nbeta\ngamma"}
 var out bytes.Buffer
 script:="delete-line folder/settings.txt\n2\nDELETE 2\nexit\n"
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(script)),&out);err!=nil{t.Fatal(err)}
 hash:=sha256.Sum256([]byte(f.previewText))
 if f.replacePath!="folder/settings.txt"||
  f.replaceText!="alpha\ngamma"||
  f.replaceSHA!=hex.EncodeToString(hash[:]){
  t.Fatalf("delete failed full-file SHA-256 CAS: %+v",f)
 }
 if got:=strings.Join(f.actions,",");got!="preview:folder/settings.txt,edit:folder/settings.txt"{
  t.Fatalf("unexpected E2EE action sequence %q",got)
 }
 if !strings.Contains(out.String(),"Dòng 2 sẽ bị xóa")||
  !strings.Contains(out.String(),"DELETE 2"){
  t.Fatalf("delete confirmation or deleted preview missing: %q",out.String())
 }
}

func TestInteractiveInsertDeleteCancelNeverWrites(t *testing.T){
 cases:=[]string{
  "insert-line a.txt\n1\nbefore\nYES 1\nexit\n",
  "insert-line a.txt\n1\nbefore\nINSERT 2\nexit\n",
  "insert-line a.txt\n0\nbefore\nINSERT 0\nexit\n",
  "insert-line a.txt\n100\nbefore\nINSERT 100\nexit\n",
  "delete-line a.txt\n1\nYES 1\nexit\n",
  "delete-line a.txt\n1\nDELETE 2\nexit\n",
  "delete-line a.txt\n0\nDELETE 0\nexit\n",
  "delete-line a.txt\n100\nDELETE 100\nexit\n",
 }
 for _,script:=range cases {
  f:=&fakeInteractiveTaskDeck{previewText:"abc\nxyz\n"}
  var out bytes.Buffer
  if err:=runNativeInteractiveConsole(context.Background(),f,
   bufio.NewReader(strings.NewReader(script)),&out);err!=nil{t.Fatal(err)}
  if f.replacePath!=""||len(f.actions)!=1||f.actions[0]!="preview:a.txt"{
   t.Fatalf("cancel or invalid line unexpectedly wrote: script=%q actions=%v",script,f.actions)
  }
 }
}

func TestInteractiveInsertDeleteDoNotUseLastQuotaActionForRead(t *testing.T){
 for _,command:=range []string{"insert-line a.txt","delete-line a.txt"} {
  f:=&fakeInteractiveTaskDeck{}
  var out bytes.Buffer
  script:=strings.Repeat("git status\n",5)+command+"\nexit\n"
  if err:=runNativeInteractiveConsole(context.Background(),f,
   bufio.NewReader(strings.NewReader(script)),&out);err!=nil{t.Fatal(err)}
  if len(f.actions)!=5||f.replacePath!=""{
   t.Fatalf("mutation %s bypassed two-action quota: %+v",command,f)
  }
 }
}

func TestInteractiveInsertDeletePathsAreConstrainedToWorkspace(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 script:="insert-line ../private\ninsert-line /etc/passwd\ndelete-line ../../secret\ndelete-line a\u202etxt\nexit\n"
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(script)),&out);err!=nil{t.Fatal(err)}
 if len(f.actions)!=0 {t.Fatalf("path traversal reached remote: %+v",f.actions)}
}


func TestInteractiveEditRangePreservesOnlySurroundingBytesAndEOL(t *testing.T) {
 cases := []struct{source string; first,last int; replacement,want string}{
  {"a\nb\nc\n",2,2,"x\ny","a\nx\ny\nc\n"},
  {"a\r\nb\r\nc\r\n",2,3,"x\ny","a\r\nx\r\ny\r\n"},
  {"a\nb",2,2,"x\ny","a\nx\ny"},
  {"a\r\nb\nc\r\n",2,2,"x\ny","a\r\nx\ny\nc\r\n"},
  {"a\nb\n",2,2,"","a\n\n"},
  {"a",1,1,"x\ny","x\ny"},
  {"a\nb",1,1,"x\n","x\n\nb"},
 }
 for _, tc := range cases {
  got,err:=consoleReplaceLineRange(tc.source,tc.first,tc.last,tc.replacement)
  if err!=nil||got!=tc.want {
   t.Errorf("replace range %d-%d in %q: got=%q err=%v, want=%q",
    tc.first,tc.last,tc.source,got,err,tc.want)
  }
 }
}

func TestInteractiveEditRangeRejectsInvalidRangesAndUntrustedContent(t *testing.T) {
 cases:=[]struct{source string; first,last int; replacement string}{
  {"",1,1,"abc"},
  {"a",0,1,"abc"},
  {"a",1,2,"abc"},
  {"a\nb",2,1,"abc"},
  {"a",1,1,"a"},
  {"a",1,1,"x\n"}, // Never change an unterminated EOF via .save.
  {"a\nb",2,2,"x\n"},
  {"a\nb",2,2,""}, // Blank last line must not change EOF newline state.
  {"a",1,1,"\x1b[31m"},
  {"a",1,1,"a\rb"},
  {"a",1,1,"a\u202eb"},
  {"a",1,1,"a\x00b"},
  {"a",1,1,string([]byte{0xff})},
  {"a",1,1,strings.Repeat("q",4097)},
  {strings.Repeat("a",4096),1,1,strings.Repeat("b",4097)},
 }
 for _,tc:=range cases {
  got,err:=consoleReplaceLineRange(tc.source,tc.first,tc.last,tc.replacement)
  if err==nil||got!="" {
   t.Errorf("unsafe range edit accepted: %d-%d sourceLen=%d replacementLen=%d",
    tc.first,tc.last,len(tc.source),len(tc.replacement))
  }
 }
}

func TestInteractiveEditRangeUsesOnlyPinnedPreviewAndHostCAS(t *testing.T) {
 f:=&fakeInteractiveTaskDeck{previewText:"one\r\ntwo\r\nthree\r\n"}
 var out bytes.Buffer
 script:="edit-range dir with spaces/config.ini\n2\n3\nalpha\nbeta\n.save-noeol\nRANGE 2-3\nexit\n"
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(script)),&out);err!=nil{t.Fatal(err)}
 hash:=sha256.Sum256([]byte(f.previewText))
 if f.replacePath!="dir with spaces/config.ini"||
  f.replaceText!="one\r\nalpha\r\nbeta\r\n"||
  f.replaceSHA!=hex.EncodeToString(hash[:]){
  t.Fatalf("range edit did not preserve CRLF or full-file SHA-CAS: %+v",f)
 }
 if got:=strings.Join(f.actions,",");got!="preview:dir with spaces/config.ini,edit:dir with spaces/config.ini"{
  t.Fatalf("unexpected E2EE dispatch: %s",got)
 }
 if !strings.Contains(out.String(),"Toàn bộ nội dung dự kiến")||
  !strings.Contains(out.String(),"3: beta")||
  !strings.Contains(out.String(),"RANGE 2-3"){
  t.Fatalf("confirmed full preview missing: %q",out.String())
 }
}

func TestInteractiveEditRangeInvalidCancelQuotaAndTraversalNeverWrite(t *testing.T) {
 cases:=[]string{
  "edit-range a.txt\n2\n1\nexit\n",
  "edit-range a.txt\n0\n2\nexit\n",
  "edit-range a.txt\n1\n4\nexit\n",
  "edit-range a.txt\n1\n2\n.cancel\nexit\n",
  "edit-range a.txt\n1\n2\nchanged\n.save-noeol\nYES\nexit\n",
  "edit-range a.txt\n1\n2\n\x1b[31m\n.save-noeol\nRANGE 1-2\nexit\n",
 }
 for _, script:=range cases {
  f:=&fakeInteractiveTaskDeck{previewText:"a\nb\nc\n"}
  var out bytes.Buffer
  if err:=runNativeInteractiveConsole(context.Background(),f,
   bufio.NewReader(strings.NewReader(script)),&out);err!=nil{t.Fatal(err)}
  if got:=strings.Join(f.actions,",");got!="preview:a.txt"||f.replacePath!=""{
   t.Fatalf("bad confirmation or range wrote to host: actions=%s",got)
  }
 }
 for _,script:=range []string{
  "edit-range ../secret\nexit\n",
  "edit-range /etc/passwd\nexit\n",
  "edit-range a\u202eb\nexit\n",
  strings.Repeat("git status\n",5)+"edit-range a.txt\nexit\n",
 }{
  f:=&fakeInteractiveTaskDeck{}
  var out bytes.Buffer
  if err:=runNativeInteractiveConsole(context.Background(),f,
   bufio.NewReader(strings.NewReader(script)),&out);err!=nil{t.Fatal(err)}
  if f.replacePath!=""{
   t.Fatalf("unsafe path or quota wrote to host: %+v",f)
  }
  if strings.Contains(script,"edit-range")&&strings.Contains(script,"git status")&&len(f.actions)!=5{
   t.Fatalf("insufficient action quota was bypassed: %+v",f.actions)
  }
 }
}


func TestInteractiveMoveLinesPreservesExactRawLineBytes(t *testing.T) {
 cases:=[]struct{source string;first,last,before int;want string}{
  {"a\nb\nc\n",2,2,1,"b\na\nc\n"},
  {"a\nb\nc\n",1,1,4,"b\nc\na\n"},
  {"a\nb\nc\nd\n",2,3,1,"b\nc\na\nd\n"},
  {"a\nb\nc\nd\n",2,3,5,"a\nd\nb\nc\n"},
  {"a\r\nb\nc\r\n",2,2,1,"b\na\r\nc\r\n"},
  {"x\r\n\r\ny\r\n",2,2,4,"x\r\ny\r\n\r\n"},
  {"a\n",1,1,1,""}, // no-op, must be denied
 }
 for _,tc:=range cases{
  got,err:=consoleMoveLines(tc.source,tc.first,tc.last,tc.before)
  if tc.want==""{
   if err==nil||got!=""{t.Errorf("no-op accepted: %q",got)}
   continue
  }
  if err!=nil||got!=tc.want{
   t.Errorf("move %d-%d before %d in %q: got=%q err=%v want=%q",
    tc.first,tc.last,tc.before,tc.source,got,err,tc.want)
  }
 }
}

func TestInteractiveMoveLinesRejectsUnsafeOrUnsupportedSource(t *testing.T){
 cases:=[]struct{source string;first,last,before int}{
  {"",1,1,1},
  {"a\nb",1,1,3}, // Unterminated final line, even if not moved
  {"a",1,1,2},
  {"a\nb\n",0,1,2},
  {"a\nb\n",2,1,1},
  {"a\nb\n",1,3,1},
  {"a\nb\n",1,1,0},
  {"a\nb\n",1,1,4},
  {"a\nb\n",1,1,1},
  {"a\nb\n",1,1,2},
  {"a\nb\n",1,2,3},
  {strings.Repeat("x",4097)+"\n",1,1,1},
 }
 for _,tc:=range cases{
  got,err:=consoleMoveLines(tc.source,tc.first,tc.last,tc.before)
  if err==nil||got!=""{
   t.Errorf("invalid source/range accepted: got=%q params=%+v",got,tc)
  }
 }
}

func TestInteractiveMoveLinesUsesPreviewAndExactCASConfirmation(t *testing.T){
 f:=&fakeInteractiveTaskDeck{previewText:"one\r\ntwo\nthree\r\n"}
 var out bytes.Buffer
 script:="move-lines dir with spaces/source.txt\n2\n2\n1\nMOVE 2-2 BEFORE 1\nexit\n"
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(script)),&out);err!=nil{t.Fatal(err)}
 sum:=sha256.Sum256([]byte(f.previewText))
 if f.replacePath!="dir with spaces/source.txt"||
  f.replaceText!="two\none\r\nthree\r\n"||
  f.replaceSHA!=hex.EncodeToString(sum[:]) {
  t.Fatalf("move must write exact raw block with SHA from authenticated preview: %+v",f)
 }
 if got:=strings.Join(f.actions,",");got!="preview:dir with spaces/source.txt,edit:dir with spaces/source.txt" {
  t.Fatalf("unexpected typed E2EE actions: %s",got)
 }
 if !strings.Contains(out.String(),"Toàn bộ nội dung dự kiến")||
  !strings.Contains(out.String(),"MOVE 2-2 BEFORE 1"){
  t.Fatalf("missing full preview and exact confirmation: %q",out.String())
 }
}

func TestInteractiveMoveLinesCancelInvalidInputsAndQuotaNeverWrite(t *testing.T){
 scripts:=[]string{
  "move-lines a.txt\n2\n1\n4\nexit\n",
  "move-lines a.txt\n0\n1\n4\nexit\n",
  "move-lines a.txt\n1\n2\n2\nexit\n",
  "move-lines a.txt\n1\n2\n4\nNO\nexit\n",
  "move-lines a.txt\n1\n2\n4\nMOVE 2-1 BEFORE 4\nexit\n",
  "move-lines a.txt\n1\n2\n4\nMOVE 1-2 BEFORE 3\nexit\n",
 }
 for _,script:=range scripts{
  f:=&fakeInteractiveTaskDeck{previewText:"a\nb\nc\n"}
  var out bytes.Buffer
  if err:=runNativeInteractiveConsole(context.Background(),f,
   bufio.NewReader(strings.NewReader(script)),&out);err!=nil{t.Fatal(err)}
  if got:=strings.Join(f.actions,",");got!="preview:a.txt"||f.replacePath!="" {
   t.Fatalf("invalid move reached write boundary: %+v, script=%q",f.actions,script)
  }
 }
 for _,script:=range []string{
  "move-lines ../private\nexit\n",
  "move-lines /etc/passwd\nexit\n",
  "move-lines a\u202eb\nexit\n",
  strings.Repeat("git status\n",5)+"move-lines a.txt\nexit\n",
 }{
  f:=&fakeInteractiveTaskDeck{}
  var out bytes.Buffer
  if err:=runNativeInteractiveConsole(context.Background(),f,
   bufio.NewReader(strings.NewReader(script)),&out);err!=nil{t.Fatal(err)}
  if f.replacePath!=""{t.Fatalf("unsafe request wrote to host: %v",f.actions)}
  if strings.Contains(script,"git status") && len(f.actions)!=5{
   t.Fatalf("move-lines bypassed two-action quota: %v",f.actions)
  }
  if !strings.Contains(script,"git status")&&len(f.actions)!=0{
   t.Fatalf("invalid path reached host: %v",f.actions)
  }
 }
}


func TestInteractiveMoveLinesKeepsWorkspaceScopedCWDAndEscapesPreview(t *testing.T) {
 f:=&fakeInteractiveTaskDeck{previewText:"one\n\x1b[31mtwo\nthree\n"}
 var out bytes.Buffer
 script:="cd folder with spaces\nmove-lines source.txt\n2\n2\n1\nMOVE 2-2 BEFORE 1\nexit\n"
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(script)),&out);err!=nil{t.Fatal(err)}
 if got:=strings.Join(f.actions,",");got!="preview:folder with spaces/source.txt,edit:folder with spaces/source.txt" {
  t.Fatalf("move-lines escaped client workspace path: %s",got)
 }
 if f.replaceText!="\x1b[31mtwo\none\nthree\n" {
  t.Fatalf("raw bytes should be preserved in authenticated host CAS: %q",f.replaceText)
 }
 if strings.Contains(out.String(),"\x1b")||!strings.Contains(out.String(),"\\u001b") {
  t.Fatalf("remote file sent raw terminal escape to operator: %q",out.String())
 }
}
