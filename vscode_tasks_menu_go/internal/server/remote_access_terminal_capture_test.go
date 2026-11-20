package server

import (
 "strings"
 "testing"
 "unicode/utf8"
)

// Terminal output must stay bounded IN MEMORY, but a connected client
// that keeps polling may read substantially more than the first 16KiB.
// The original task capture still preserves its first-window behavior.
func TestRemoteTerminalRollingCaptureKeepsServingFutureOutput(t *testing.T){
 capture:=newRemoteTerminalCapture()
 cursor:=0
 for i:=0;i<7;i++{
  chunk:=strings.Repeat(string(rune('A'+i)),5000)
  if n,err:=capture.Write([]byte(chunk));err!=nil||n!=len(chunk){
   t.Fatalf("terminal writer failed: %d %v",n,err)
  }
  var seen strings.Builder
  for page:=0;page<100;page++{
   nextText,next,truncated,more:=capture.next(cursor)
   if truncated{t.Fatalf("no unread bytes were lost on page %d of segment %d",page,i)}
   if next<=cursor{t.Fatalf("cursor did not advance: %d -> %d",cursor,next)}
   seen.WriteString(nextText)
   cursor=next
   if !more{break}
   if page==99{t.Fatal("terminal output pagination did not finish")}
  }
  if got:=seen.String();got!=chunk{
   t.Fatalf("segment %d lost output: received %d expected %d",i,len(got),len(chunk))
  }
  capture.mu.Lock()
  size,base:=len(capture.data),capture.rollingBase
  capture.mu.Unlock()
  if size>remoteLongTaskMaxCaptureBytes{
   t.Fatalf("terminal capture exceeded memory limit: %d",size)
  }
  if i==6&&base==0{t.Fatal("rolling capture never reclaimed old bytes")}
 }
 if cursor!=7*5000{t.Fatalf("absolute E2EE cursor reset after sliding: %d",cursor)}
}

func TestRemoteTerminalRollingCaptureReportsDroppedUnreadOutput(t *testing.T){
 capture:=newRemoteTerminalCapture()
 full:=strings.Repeat("a",42000)+"TAIL-MARKER\n"
 if _,err:=capture.Write([]byte(full));err!=nil{t.Fatal(err)}
 var output strings.Builder
 cursor:=0
 first:=true
 for pages:=0;pages<200;pages++{
  chunk,next,truncated,more:=capture.next(cursor)
  if first&&!truncated{t.Fatal("overwritten unread output was not signalled")}
  if next<cursor{t.Fatal("rolling cursor moved backwards")}
  output.WriteString(chunk)
  cursor=next
  first=false
  if !more{break}
  if pages==199{t.Fatal("rolling capture tail never completed")}
 }
 if got:=output.String();got!=full[len(full)-remoteLongTaskMaxCaptureBytes:]{
  t.Fatalf("incorrect retained output tail length=%d",len(got))
 }
 if !strings.HasSuffix(output.String(),"TAIL-MARKER\n"){
  t.Fatal("last PTY output was discarded after the original capture filled")
 }
 // The capture is allowed to drop only bytes which were actually read
 // before rollover; an up-to-date cursor should not signal data loss.
 next,after,truncated,more:=capture.next(cursor)
 if next!=""||after!=cursor||truncated||more{
  t.Fatalf("already consumed rolling tail was replayed: %q %d %t %t",next,after,truncated,more)
 }
}

func TestRemoteTerminalRollingCaptureUTF8BoundariesAndFinalFreeze(t *testing.T){
 capture:=newRemoteTerminalCapture()
 if _,err:=capture.Write([]byte{'A',0xE2});err!=nil{t.Fatal(err)}
 first,cursor,truncated,more:=capture.next(0)
 if first!="A"||truncated||more{
  t.Fatalf("partial UTF-8 sequence was emitted: %q %t %t",first,truncated,more)
 }
 if _,err:=capture.Write([]byte{0x82,0xAC,'\r','\n','B',0x1b});err!=nil{t.Fatal(err)}
 text,after,truncated,more:=capture.next(cursor)
 if text!="€\nB"||truncated||more||after!=len("A€\nB"){
  t.Fatalf("fragmented Unicode/control sanitation failed: %q %d %t %t",text,after,truncated,more)
 }
 if _,err:=capture.Write([]byte(strings.Repeat("中",8000)));err!=nil{t.Fatal(err)}
 piece,next,loss,_:=capture.next(after)
 if !loss||!utf8.ValidString(piece)||next<=after{
  t.Fatalf("rolling UTF-8 tail split a rune or lost overrun signal: %q %d %t",piece,next,loss)
 }
 capture.finishRemoteTerminal(false)
 capture.mu.Lock()
 frozen:=string(capture.data)
 base:=capture.rollingBase
 capture.mu.Unlock()
 if _,err:=capture.Write([]byte("UNAUTHORIZED-LATE-OUTPUT"));err!=nil{t.Fatal(err)}
 capture.mu.Lock()
 stable:=string(capture.data)==frozen&&capture.rollingBase==base
 capture.mu.Unlock()
 if !stable{t.Fatal("PTY reader changed already finalized transcript")}
 incomplete:=newRemoteTerminalCapture()
 if _,err:=incomplete.Write([]byte{0xe2,0x82});err!=nil{t.Fatal(err)}
 incomplete.finishRemoteTerminal(false)
 _,_,truncated,_=incomplete.next(0)
 if !truncated{t.Fatal("terminal ended with an incomplete UTF-8 sequence without truncation")}
}

func TestRemoteTerminalRollingCapturePreservesDeferredCursorAfterCompaction(t *testing.T){
 capture:=newRemoteTerminalCapture()
 if _,err:=capture.Write([]byte(strings.Repeat("x",300)));err!=nil{t.Fatal(err)}
 job:=&remoteTerminalJob{capture:capture,state:"running"}
 session:=&remoteTaskDeckSession{terminal:job}
 first,commit,err:=session.remoteTerminalSnapshotWithMode(true)
 if err!=nil||commit==nil||len(first.Output)!=remoteLongTaskMaxChunkBytes{
  t.Fatalf("invalid deferred output: %+v %v",first,err)
 }
 if _,err:=capture.Write([]byte(strings.Repeat("y",30000)+"FINAL"));err!=nil{t.Fatal(err)}
 commit() // original AEAD Seal finally succeeded after host output rolled
 job.mu.Lock()
 cursor:=job.cursor
 job.mu.Unlock()
 if cursor!=remoteLongTaskMaxChunkBytes{
  t.Fatalf("deferred E2EE response committed an incorrect cursor: %d",cursor)
 }
 next,err:=session.remoteTerminalSnapshot()
 if err!=nil||!next.Truncated||!strings.Contains(next.Output,"y"){
  t.Fatalf("next E2EE Poll omitted overwritten-prefix marker: %+v %v",next,err)
 }
 job.mu.Lock()
 now:=job.cursor
 job.mu.Unlock()
 if now<=cursor{t.Fatalf("rolling E2EE cursor did not advance: %d -> %d",cursor,now)}
}
