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

func TestInteractiveFindLinesValidatesCompleteResultAndQuerySHA(t *testing.T){
 const term="needle"
 fileSHA:=strings.Repeat("a",64)
 sum:=sha256.Sum256([]byte(term))
 report:="FILE_SHA256 "+fileSHA+"\nTERM_SHA256 "+hex.EncodeToString(sum[:])+
  "\nTOTAL_LINES 90\nMATCHES 2\n-- COMPLETE MATCHING LINES --\n12: a needle line\n80: needle again\n"
 hash,err:=consoleFindLinesMetadata(report,term)
 if err!=nil||hash!=fileSHA{t.Fatalf("valid complete match rejected: %q %v",hash,err)}
 for _,bad:=range []string{
  strings.Replace(report,"MATCHES 2","MATCHES 3",1),
  strings.Replace(report,"MATCHES 2","MATCHES 0",1),
  strings.Replace(report,"TERM_SHA256 "+hex.EncodeToString(sum[:]),"TERM_SHA256 "+strings.Repeat("b",64),1),
  strings.Replace(report,"FILE_SHA256 "+fileSHA,"FILE_SHA256 "+strings.Repeat("A",64),1),
  strings.Replace(report,"12: a needle line","12: not a match",1),
  strings.Replace(report,"80: needle again","11: needle again",1),
  strings.Replace(report,"TOTAL_LINES 90","TOTAL_LINES 50",1),
  strings.TrimSuffix(report,"\n"),
  report+"EXTRA",
  strings.Repeat("x",4097),
 }{
  if _,err:=consoleFindLinesMetadata(bad,term);err==nil{
   t.Fatalf("forged partial match transcript accepted: %q",bad)
  }
 }
 absent:="FILE_SHA256 "+fileSHA+"\nTERM_SHA256 "+hex.EncodeToString(sum[:])+
  "\nTOTAL_LINES 1\nMATCHES 0\n-- COMPLETE MATCHING LINES --\n"
 if _,err:=consoleFindLinesMetadata(absent,term);err!=nil{t.Fatalf("zero hits rejected: %v",err)}
}

func TestInteractiveFindLinesRoutesOnlyLiteralE2EEAndEscapesTerminal(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 input:="find-lines src/file with spaces.go :: needle\nexit\n"
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(input)),&out);err!=nil{t.Fatal(err)}
 if got:=strings.Join(f.actions,",");got!="find-lines:src/file with spaces.go:needle"{
  t.Fatalf("unexpected remote action %q",got)
 }
 if !strings.Contains(out.String(),"1: needle")||!strings.Contains(out.String(),"FILE_SHA256"){
  t.Fatalf("search hits omitted: %s",out.String())
 }
}

func TestInteractiveFindLinesDeniedBeforeNetworkForMalformedInput(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 for _,input:=range []string{
  "find-lines a.go","find-lines ../secret :: needle",
  "find-lines .git/config :: needle","find-lines a.go :: ",
  "find-lines a.go ::  leading","find-lines a.go :: hi; git push\n",
  "find-lines a.go needle","find-lines /etc/passwd :: x",
 }{
  // Shell punctuation inside a *literal search term* is allowed, since
  // it is never executed. Test independent path/argument corruption.
  if strings.Contains(input,"hi; git push"){continue}
  script:=strings.TrimSuffix(input,"\n")+"\nexit\n"
  if err:=runNativeInteractiveConsole(context.Background(),f,
   bufio.NewReader(strings.NewReader(script)),&out);err!=nil{t.Fatal(err)}
 }
 if len(f.actions)!=0{
  t.Fatalf("malformed find-lines sent network requests: %v",f.actions)
 }
}

func TestInteractiveFindLinesPreventsMixingOtherSourceVersion(t *testing.T){
 f:=&fakeInteractiveTaskDeck{linePageVersions:[]string{strings.Repeat("a",64),strings.Repeat("b",64)}}
 var out bytes.Buffer
 input:="view-lines a.go 1\nfind-lines a.go :: example\nexit\n"
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(input)),&out);err!=nil{t.Fatal(err)}
 if len(f.actions)!=2||!strings.Contains(out.String(),"đã đổi SHA"){
  t.Fatalf("mixed-version find exposed source: actions=%v output=%q",f.actions,out.String())
 }
}

func TestInteractiveFindLinesRespectsShortSessionQuota(t *testing.T){
 f:=&fakeInteractiveTaskDeck{}
 var out bytes.Buffer
 input:=strings.Repeat("find-lines a.go :: needle\n",8)+"exit\n"
 if err:=runNativeInteractiveConsole(context.Background(),f,
  bufio.NewReader(strings.NewReader(input)),&out);err!=nil{t.Fatal(err)}
 if len(f.actions)!=6||!strings.Contains(out.String(),"Phiên đã dùng hết"){
  t.Fatalf("overquota search: actions=%v output=%q",f.actions,out.String())
 }
}
